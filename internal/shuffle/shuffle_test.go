package shuffle

import (
	"math"
	"sync"
	"testing"
)

// TestAllPermutationsAppearUniformly 洗一副 4 張的小牌很多次，檢查
// 24 種排列的出現次數是否夠接近平均 —— 洗牌若有偏差會在這裡現形。
func TestAllPermutationsAppearUniformly(t *testing.T) {
	const (
		size  = 4
		perms = 24 // 4!
		runs  = 240_000
	)

	counts := make(map[[size]int]int, perms)
	var s Crypto
	for range runs {
		deck := [size]int{0, 1, 2, 3}
		s.Shuffle(size, func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		counts[deck]++
	}

	if len(counts) != perms {
		t.Fatalf("只出現 %d 種排列，應該有 %d 種", len(counts), perms)
	}

	// 用卡方檢定看分布是否偏離均勻。自由度 23 時，
	// 卡方值超過 52.6 的機率不到萬分之一，拿它當門檻。
	expected := float64(runs) / perms
	var chiSq float64
	for _, got := range counts {
		d := float64(got) - expected
		chiSq += d * d / expected
	}
	const threshold = 52.6
	if chiSq > threshold {
		t.Errorf("排列分布不夠均勻：卡方值 %.1f 超過門檻 %.1f", chiSq, threshold)
	}
}

// TestNaiveShuffleWouldBeBiased 說明為什麼一定要用 Fisher–Yates。
//
// 有個看起來很合理但其實錯誤的做法：「從第一張開始，每張牌都跟*任意*
// 位置的牌交換，跑完 n 次」。它產生 n^n 條等機率的路徑，但排列只有 n!
// 種，而 n^n 不能被 n! 整除，所以有些排列必然比別的常出現。
//
// 這個測試先證明那個做法確實有偏差，再確認我們的實作沒有 ——
// 免得日後有人「簡化」成那樣。
func TestNaiveShuffleWouldBeBiased(t *testing.T) {
	const (
		size = 4
		runs = 200_000
	)

	// 錯誤做法：交換對象從全部位置裡挑。
	naive := make(map[[size]int]int)
	for range runs {
		deck := [size]int{0, 1, 2, 3}
		for i := range size {
			j := int(randBelow(size)) // 從 [0, size) 挑 —— 這就是錯的地方
			deck[i], deck[j] = deck[j], deck[i]
		}
		naive[deck]++
	}

	// 正確做法：交換對象只從尚未定案的 [0, i] 裡挑。
	correct := make(map[[size]int]int)
	var s Crypto
	for range runs {
		deck := [size]int{0, 1, 2, 3}
		s.Shuffle(size, func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		correct[deck]++
	}

	spread := func(m map[[size]int]int) float64 {
		lo, hi := runs, 0
		for _, c := range m {
			lo = min(lo, c)
			hi = max(hi, c)
		}
		return float64(hi) / float64(lo)
	}

	naiveSpread, correctSpread := spread(naive), spread(correct)

	// 錯誤做法在 4 張牌時，最常見的排列約是最罕見的 1.9 倍。
	if naiveSpread < 1.5 {
		t.Errorf("預期天真做法會有明顯偏差，實際最多/最少只有 %.2f 倍 —— "+
			"這個測試可能失去意義了", naiveSpread)
	}
	// 我們的實作應該幾乎沒有偏差。
	if correctSpread > 1.1 {
		t.Errorf("Fisher–Yates 不該有明顯偏差，實際最多/最少 %.2f 倍", correctSpread)
	}
	t.Logf("天真做法偏差 %.2f 倍，Fisher–Yates 偏差 %.2f 倍", naiveSpread, correctSpread)
}

// TestEachCardReachesEachPosition 驗證洗一副 52 張的牌時，
// 每張牌都有機會落到每個位置，而且次數接近平均。
func TestEachCardReachesEachPosition(t *testing.T) {
	const (
		size = 52
		runs = 20_000
	)

	// counts[card][pos] 是某張牌落在某個位置的次數。
	counts := make([][]int, size)
	for i := range counts {
		counts[i] = make([]int, size)
	}

	var s Crypto
	deck := make([]int, size)
	for range runs {
		for i := range deck {
			deck[i] = i
		}
		s.Shuffle(size, func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		for pos, card := range deck {
			counts[card][pos]++
		}
	}

	// 每張牌落在每個位置的期望次數是 runs/size；
	// 容許 ±35% 的浮動，明顯的偏差（例如某張牌永遠不到某處）才會被抓到。
	expected := float64(runs) / size
	tolerance := expected * 0.35
	for card := range counts {
		for pos, got := range counts[card] {
			if math.Abs(float64(got)-expected) > tolerance {
				t.Errorf("牌 %d 落在位置 %d 共 %d 次，期望約 %.0f 次",
					card, pos, got, expected)
			}
		}
	}
}

// TestShuffleIsConcurrencySafe 讓多條 goroutine 同時洗牌。
// Crypto 沒有共用的可變狀態，所以不該出現資料競爭。
// 這正是不用 math/rand 的原因之一：*rand.Rand 併發使用會有競爭。
func TestShuffleIsConcurrencySafe(t *testing.T) {
	var s Crypto
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				deck := [13]int{}
				s.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
			}
		}()
	}
	wg.Wait()
}

// TestShuffleKeepsAllElements 驗證洗牌只是重排，不會弄丟或複製元素。
func TestShuffleKeepsAllElements(t *testing.T) {
	var s Crypto
	for size := range 60 {
		deck := make([]int, size)
		for i := range deck {
			deck[i] = i
		}
		s.Shuffle(size, func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

		seen := make(map[int]bool, size)
		for _, v := range deck {
			if seen[v] {
				t.Fatalf("size %d: 元素 %d 出現兩次", size, v)
			}
			if v < 0 || v >= size {
				t.Fatalf("size %d: 出現了範圍外的值 %d", size, v)
			}
			seen[v] = true
		}
		if len(seen) != size {
			t.Fatalf("size %d: 洗完只剩 %d 個元素", size, len(seen))
		}
	}
}

// TestRandBelowCoversWholeRange 驗證 randBelow 會覆蓋 [0, n) 的每個值。
func TestRandBelowCoversWholeRange(t *testing.T) {
	for _, n := range []uint64{1, 2, 3, 7, 52} {
		seen := make(map[uint64]bool, n)
		// 抽夠多次，每個值都該出現過。
		for range int(n) * 200 {
			v := randBelow(n)
			if v >= n {
				t.Fatalf("randBelow(%d) 回傳了超出範圍的 %d", n, v)
			}
			seen[v] = true
		}
		if uint64(len(seen)) != n {
			t.Errorf("randBelow(%d) 只產生了 %d 種值", n, len(seen))
		}
	}
}

// TestSeededIsReproducible 驗證測試用的洗牌器同種子會給出相同結果，
// 這是重現牌局的基礎。
func TestSeededIsReproducible(t *testing.T) {
	shuffleOnce := func() [10]int {
		deck := [10]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		s := Seeded(42)
		s.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		return deck
	}
	if a, b := shuffleOnce(), shuffleOnce(); a != b {
		t.Errorf("同一個種子應該給出相同排列：%v vs %v", a, b)
	}
}
