package match

import (
	"math"
	"slices"
	"testing"

	"playground/internal/games/bigtwo/rules"
	"playground/internal/shuffle"
)

// TestDealIsUnbiased 用真正的洗牌器發很多局，檢查發牌沒有偏差：
// 每張牌落到每個座位的機率都該接近 1/4。
//
// 洗牌器本身的均勻性在 shuffle 套件裡驗證過了，這裡驗證的是
// 「發牌這段程式有沒有把均勻性破壞掉」—— 例如切錯區間或排序時弄錯。
func TestDealIsUnbiased(t *testing.T) {
	if testing.Short() {
		t.Skip("統計檢定需要大量樣本，跳過短測試")
	}

	const runs = 20_000
	names := [NumPlayers]string{"A", "B", "C", "D"}
	var s shuffle.Crypto

	// seatOf[card][seat] 是某張牌發給某個座位的次數。
	seatOf := make(map[rules.Card][NumPlayers]int, 52)
	for range runs {
		m := New(names, s)
		for _, p := range m.Players {
			for _, c := range p.Hand {
				counts := seatOf[c]
				counts[p.Seat]++
				seatOf[c] = counts
			}
		}
	}

	if len(seatOf) != 52 {
		t.Fatalf("只發出 %d 種牌，應該是 52 種", len(seatOf))
	}

	// 每張牌落在每個座位的期望次數是 runs/4，容許 ±12% 的浮動。
	expected := float64(runs) / NumPlayers
	tolerance := expected * 0.12
	for card, counts := range seatOf {
		total := 0
		for seat, got := range counts {
			total += got
			if math.Abs(float64(got)-expected) > tolerance {
				t.Errorf("%v 發給座位 %d 共 %d 次，期望約 %.0f 次",
					card, seat, got, expected)
			}
		}
		// 每一局這張牌都必須剛好發出去一次。
		if total != runs {
			t.Errorf("%v 總共發出 %d 次，應該是 %d 次", card, total, runs)
		}
	}
}

// TestFirstPlayerIsUnbiased 驗證誰先出牌（誰拿到梅花 3）也是均勻的。
// 先手在大老二有實質優勢，不該固定落在某個座位。
func TestFirstPlayerIsUnbiased(t *testing.T) {
	if testing.Short() {
		t.Skip("統計檢定需要大量樣本，跳過短測試")
	}

	const runs = 20_000
	names := [NumPlayers]string{"A", "B", "C", "D"}
	var s shuffle.Crypto

	var first [NumPlayers]int
	for range runs {
		m := New(names, s)
		first[m.Turn]++
	}

	expected := float64(runs) / NumPlayers
	tolerance := expected * 0.08
	for seat, got := range first {
		if math.Abs(float64(got)-expected) > tolerance {
			t.Errorf("座位 %d 先出牌 %d 次，期望約 %.0f 次", seat, got, expected)
		}
	}
}

// TestDealIsComplete 驗證每一局都發滿 52 張、不重不漏，
// 且每家剛好 13 張。用真正的洗牌器多跑幾局。
func TestDealIsComplete(t *testing.T) {
	names := [NumPlayers]string{"A", "B", "C", "D"}
	var s shuffle.Crypto

	for run := range 200 {
		m := New(names, s)

		seen := make(map[rules.Card]bool, 52)
		for _, p := range m.Players {
			if len(p.Hand) != CardsPerHand {
				t.Fatalf("第 %d 局：座位 %d 拿到 %d 張", run, p.Seat, len(p.Hand))
			}
			for _, c := range p.Hand {
				if seen[c] {
					t.Fatalf("第 %d 局：%v 發了兩次", run, c)
				}
				seen[c] = true
			}
			// 手牌必須排好序，前端才能穩定顯示。
			for i := 1; i < len(p.Hand); i++ {
				if !p.Hand[i-1].Less(p.Hand[i]) {
					t.Fatalf("第 %d 局：座位 %d 的手牌沒排序", run, p.Seat)
				}
			}
		}
		if len(seen) != 52 {
			t.Fatalf("第 %d 局：只發出 %d 張", run, len(seen))
		}
		// 持有梅花 3 的人必須先出。
		if !slices.Contains(m.Players[m.Turn].Hand, rules.ClubThree) {
			t.Fatalf("第 %d 局：先出牌的座位 %d 手上沒有梅花 3", run, m.Turn)
		}
	}
}
