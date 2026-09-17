package game

import (
	"math/rand"
	"testing"
)

// TestBeatsIsAntisymmetric 窮舉所有牌型兩兩比較，驗證大小關係不會自相矛盾：
// 兩組牌不可能互相壓過對方，一組牌也不可能壓過自己。
// 這是比大小邏輯最根本的性質，任何順位或花色的錯誤都會在這裡現形。
func TestBeatsIsAntisymmetric(t *testing.T) {
	combos := sampleCombos(t)

	for i, a := range combos {
		if a.Beats(a) {
			t.Errorf("%v 壓過了自己", a.Cards)
		}
		for j, b := range combos {
			if i == j {
				continue
			}
			if a.Beats(b) && b.Beats(a) {
				t.Errorf("%v 與 %v 互相壓過對方", a.Cards, b.Cards)
			}
		}
	}
}

// TestBeatsIsTransitive 驗證大小關係可以遞移：a>b 且 b>c 就必須 a>c。
// 少了這個性質，同一手牌的輸贏會取決於出牌順序。
func TestBeatsIsTransitive(t *testing.T) {
	combos := sampleCombos(t)

	for _, a := range combos {
		for _, b := range combos {
			if !a.Beats(b) {
				continue
			}
			for _, c := range combos {
				if b.Beats(c) && !a.Beats(c) {
					t.Errorf("遞移性被打破：%v > %v > %v，但 %v 壓不過 %v",
						a.Cards, b.Cards, c.Cards, a.Cards, c.Cards)
				}
			}
		}
	}
}

// TestEveryFiveCardHandClassifies 窮舉整副牌所有的五張組合（2,598,960 種），
// 驗證每一手都能被穩定歸類，而且不會 panic。這同時也確認了合法五張牌型的
// 實際數量與理論值相符。
func TestEveryFiveCardHandClassifies(t *testing.T) {
	if testing.Short() {
		t.Skip("窮舉 260 萬種組合，跳過短測試")
	}

	deck := NewDeck()
	counts := make(map[ComboType]int)

	var hand [5]Card
	var walk func(start, depth int)
	walk = func(start, depth int) {
		if depth == 5 {
			combo, err := NewCombo(hand[:])
			if err != nil {
				counts[Invalid]++
				return
			}
			counts[combo.Type]++
			return
		}
		for i := start; i <= len(deck)-(5-depth); i++ {
			hand[depth] = deck[i]
			walk(i+1, depth+1)
		}
	}
	walk(0, 0)

	// 理論值：
	// 同花順 —— 10 種順位 × 4 花色 = 40
	// 鐵支   —— 13 種點數 × 48 張雜牌 = 624
	// 葫蘆   —— 13×C(4,3) × 12×C(4,2) = 13×4 × 12×6 = 3744
	// 順子   —— 10 種順位 × 4^5 組花色 − 40 個同花順 = 10240 − 40 = 10200
	want := map[ComboType]int{
		StraightFlush: 40,
		FourOfAKind:   624,
		FullHouse:     3744,
		Straight:      10200,
	}
	for typ, n := range want {
		if counts[typ] != n {
			t.Errorf("%v 有 %d 種，理論上應該是 %d 種", typ, counts[typ], n)
		}
	}

	total := 0
	for _, n := range counts {
		total += n
	}
	if want := 2598960; total != want {
		t.Errorf("五張組合共 %d 種，應該是 %d 種", total, want)
	}
}

// TestStraightFlushBeatsEveryFourOfAKind 驗證規格第 10 條：
// 所有同花順，不論大小，都大於任何鐵支。
func TestStraightFlushBeatsEveryFourOfAKind(t *testing.T) {
	var flushes, quads []Combo
	for _, c := range sampleCombos(t) {
		switch c.Type {
		case StraightFlush:
			flushes = append(flushes, c)
		case FourOfAKind:
			quads = append(quads, c)
		}
	}
	if len(flushes) == 0 || len(quads) == 0 {
		t.Fatal("樣本裡缺少同花順或鐵支")
	}

	for _, sf := range flushes {
		for _, q := range quads {
			if !sf.Beats(q) {
				t.Errorf("同花順 %v 應該壓過鐵支 %v", sf.Cards, q.Cards)
			}
			if q.Beats(sf) {
				t.Errorf("鐵支 %v 不該壓過同花順 %v", q.Cards, sf.Cards)
			}
		}
	}
}

// TestSpecialsBeatEveryNormalCombo 驗證規格第 20 條：
// 鐵支與同花順可以壓過任何普通牌型，不受張數限制。
func TestSpecialsBeatEveryNormalCombo(t *testing.T) {
	var specials, normals []Combo
	for _, c := range sampleCombos(t) {
		if c.Type.special() {
			specials = append(specials, c)
		} else {
			normals = append(normals, c)
		}
	}

	for _, s := range specials {
		for _, n := range normals {
			if !s.Beats(n) {
				t.Errorf("%v %v 應該壓過 %v %v", s.Type, s.Cards, n.Type, n.Cards)
			}
			if n.Beats(s) {
				t.Errorf("%v %v 不該壓過 %v %v", n.Type, n.Cards, s.Type, s.Cards)
			}
		}
	}
}

// TestNormalCombosNeverCrossType 驗證規格第 20 條的另一半：
// 普通牌型之間沒有跨牌型的大小關係。
func TestNormalCombosNeverCrossType(t *testing.T) {
	var normals []Combo
	for _, c := range sampleCombos(t) {
		if !c.Type.special() {
			normals = append(normals, c)
		}
	}

	for _, a := range normals {
		for _, b := range normals {
			if a.Type == b.Type {
				continue
			}
			if a.Beats(b) {
				t.Errorf("%v %v 不該壓得過不同牌型的 %v %v", a.Type, a.Cards, b.Type, b.Cards)
			}
		}
	}
}

// TestNewComboDoesNotAliasInput 驗證 NewCombo 會自己持有一份牌，
// 呼叫端之後改動原本的 slice 不該影響已建立的牌型。
func TestNewComboDoesNotAliasInput(t *testing.T) {
	cards := hand(Five, Clubs, Five, Hearts)
	combo, err := NewCombo(cards)
	if err != nil {
		t.Fatalf("NewCombo 出錯: %v", err)
	}
	before := combo.Cards[0]

	cards[0] = Card{Rank: Two, Suit: Spades} // 改動原本的輸入
	if combo.Cards[0] != before {
		t.Error("改動輸入的 slice 影響到已建立的 Combo")
	}
}

// TestNewComboIntoReusesBuffer 驗證 NewComboInto 會沿用緩衝區，
// 而 Clone 出來的結果不受後續呼叫影響。
func TestNewComboIntoReusesBuffer(t *testing.T) {
	scratch := make([]Card, 2)

	first, err := NewComboInto(scratch, hand(Five, Clubs, Five, Hearts))
	if err != nil {
		t.Fatalf("NewComboInto 出錯: %v", err)
	}
	kept := first.Clone()

	// 第二次呼叫會覆蓋同一塊緩衝區。
	if _, err := NewComboInto(scratch, hand(King, Clubs, King, Hearts)); err != nil {
		t.Fatalf("NewComboInto 出錯: %v", err)
	}
	if kept.Cards[0].Rank != Five {
		t.Error("Clone 出來的結果被後續呼叫覆蓋了")
	}
}

// TestInvalidSizesRejected 驗證所有不合法的張數都會被拒絕。
// 合法的只有 1、2、5 張。
func TestInvalidSizesRejected(t *testing.T) {
	deck := NewDeck()
	for size := 0; size <= 13; size++ {
		if size == 1 || size == 2 || size == 5 {
			continue
		}
		if _, err := NewCombo(deck[:size]); err == nil {
			t.Errorf("%d 張不該是合法牌型", size)
		}
	}
}

// TestRandomFiveCardHandsNeverPanic 用隨機五張牌大量測試，
// 確認判斷邏輯對任何輸入都穩定，而且與排序無關。
func TestRandomFiveCardHandsNeverPanic(t *testing.T) {
	deck := NewDeck()
	r := rand.New(rand.NewSource(42))

	for range 20000 {
		r.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		pick := append([]Card(nil), deck[:5]...)

		first, err1 := NewCombo(pick)

		// 同一手牌換個順序送進去，結果必須完全相同。
		r.Shuffle(len(pick), func(i, j int) { pick[i], pick[j] = pick[j], pick[i] })
		second, err2 := NewCombo(pick)

		if (err1 == nil) != (err2 == nil) {
			t.Fatalf("同一手牌因排列不同而結果不一致: %v", pick)
		}
		if err1 == nil && first.Type != second.Type {
			t.Fatalf("同一手牌因排列不同被判成 %v 與 %v: %v", first.Type, second.Type, pick)
		}
		if err1 == nil && (first.Beats(second) || second.Beats(first)) {
			t.Fatalf("同一手牌的兩種排列互有大小: %v", pick)
		}
	}
}

// sampleCombos 產生一組涵蓋各種牌型與邊界的樣本，供性質測試使用。
func sampleCombos(t *testing.T) []Combo {
	t.Helper()

	raw := [][]Card{
		// 單張：最小、最大、同點數不同花色
		hand(Three, Clubs), hand(Three, Spades),
		hand(Two, Clubs), hand(Two, Spades),
		hand(Ace, Hearts),
		// 對子：最小、最大、同點數不同花色組合
		hand(Three, Clubs, Three, Diamonds),
		hand(Three, Hearts, Three, Spades),
		hand(Two, Clubs, Two, Diamonds),
		hand(Two, Hearts, Two, Spades),
		// 順子：最小順位 A2345、最大順位 23456，以及中間幾個
		hand(Ace, Clubs, Two, Clubs, Three, Clubs, Four, Clubs, Five, Diamonds),
		hand(Ace, Spades, Two, Hearts, Three, Hearts, Four, Hearts, Five, Hearts),
		hand(Three, Clubs, Four, Clubs, Five, Clubs, Six, Clubs, Seven, Diamonds),
		hand(Ten, Clubs, Jack, Clubs, Queen, Clubs, King, Clubs, Ace, Diamonds),
		hand(Two, Clubs, Three, Diamonds, Four, Diamonds, Five, Diamonds, Six, Diamonds),
		hand(Two, Spades, Three, Hearts, Four, Hearts, Five, Hearts, Six, Hearts),
		// 葫蘆：最小與最大
		hand(Three, Clubs, Three, Diamonds, Three, Hearts, Four, Clubs, Four, Diamonds),
		hand(Two, Clubs, Two, Diamonds, Two, Hearts, Ace, Clubs, Ace, Diamonds),
		// 鐵支：最小與最大
		hand(Three, Clubs, Three, Diamonds, Three, Hearts, Three, Spades, Four, Clubs),
		hand(Two, Clubs, Two, Diamonds, Two, Hearts, Two, Spades, Three, Clubs),
		// 同花順：最小順位、最大順位、不同花色
		hand(Ace, Clubs, Two, Clubs, Three, Clubs, Four, Clubs, Five, Clubs),
		hand(Ace, Spades, Two, Spades, Three, Spades, Four, Spades, Five, Spades),
		hand(Six, Hearts, Seven, Hearts, Eight, Hearts, Nine, Hearts, Ten, Hearts),
		hand(Six, Spades, Seven, Spades, Eight, Spades, Nine, Spades, Ten, Spades),
		hand(Two, Clubs, Three, Clubs, Four, Clubs, Five, Clubs, Six, Clubs),
		hand(Two, Spades, Three, Spades, Four, Spades, Five, Spades, Six, Spades),
	}

	combos := make([]Combo, 0, len(raw))
	for _, cards := range raw {
		c, err := NewCombo(cards)
		if err != nil {
			t.Fatalf("樣本 %v 應該是合法牌型: %v", cards, err)
		}
		combos = append(combos, c)
	}
	return combos
}
