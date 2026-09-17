package rules

import "testing"

// hand 讓測試用簡短寫法描述一手牌，例如 hand(Three, Clubs, Four, Clubs)。
func hand(parts ...any) []Card {
	if len(parts)%2 != 0 {
		panic("hand 需要成對的 rank 與 suit")
	}
	cards := make([]Card, 0, len(parts)/2)
	for i := 0; i < len(parts); i += 2 {
		cards = append(cards, Card{Rank: parts[i].(Rank), Suit: parts[i+1].(Suit)})
	}
	return cards
}

// mustCombo 在測試中組出一個必定合法的牌型。
func mustCombo(t *testing.T, cards []Card) Combo {
	t.Helper()
	c, err := NewCombo(cards)
	if err != nil {
		t.Fatalf("NewCombo(%v) 出錯 = %v", cards, err)
	}
	return c
}

func TestNewComboType(t *testing.T) {
	tests := []struct {
		name  string
		cards []Card
		want  ComboType
	}{
		{"單張", hand(Two, Spades), Single},
		{"對子", hand(Five, Clubs, Five, Hearts), Pair},
		{"順子", hand(Three, Clubs, Four, Hearts, Five, Spades, Six, Clubs, Seven, Diamonds), Straight},
		{"最小順子 A2345", hand(Ace, Clubs, Two, Hearts, Three, Spades, Four, Clubs, Five, Diamonds), Straight},
		{"最大順子 23456", hand(Two, Clubs, Three, Hearts, Four, Spades, Five, Clubs, Six, Diamonds), Straight},
		{"順子 10JQKA", hand(Ten, Clubs, Jack, Hearts, Queen, Spades, King, Clubs, Ace, Diamonds), Straight},
		{"葫蘆", hand(Four, Clubs, Four, Hearts, Four, Spades, Nine, Clubs, Nine, Hearts), FullHouse},
		{"鐵支", hand(Six, Clubs, Six, Diamonds, Six, Hearts, Six, Spades, King, Clubs), FourOfAKind},
		{"同花順", hand(Four, Hearts, Five, Hearts, Six, Hearts, Seven, Hearts, Eight, Hearts), StraightFlush},
		{"同花順 A2345", hand(Ace, Spades, Two, Spades, Three, Spades, Four, Spades, Five, Spades), StraightFlush},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustCombo(t, tt.cards)
			if got.Type != tt.want {
				t.Errorf("牌型 = %v, 想要 %v", got.Type, tt.want)
			}
		})
	}
}

func TestNewComboInvalid(t *testing.T) {
	tests := []struct {
		name  string
		cards []Card
	}{
		{"空牌", nil},
		{"兩張點數不同", hand(Five, Clubs, Six, Hearts)},
		{"三條不是合法牌型", hand(Five, Clubs, Five, Hearts, Five, Spades)},
		{"四張不是合法牌型", hand(Five, Clubs, Five, Hearts, Five, Spades, Five, Diamonds)},
		{"普通同花不是合法牌型", hand(Three, Spades, Seven, Spades, Nine, Spades, Jack, Spades, King, Spades)},
		{"五張散牌", hand(Three, Clubs, Six, Hearts, Nine, Spades, Jack, Clubs, King, Diamonds)},
		{"重複的牌", hand(Five, Clubs, Five, Clubs)},
		{"JQKA2 不成立", hand(Jack, Clubs, Queen, Hearts, King, Spades, Ace, Clubs, Two, Diamonds)},
		{"QKA23 不成立", hand(Queen, Clubs, King, Hearts, Ace, Spades, Two, Clubs, Three, Diamonds)},
		{"KA234 不成立", hand(King, Clubs, Ace, Hearts, Two, Spades, Three, Clubs, Four, Diamonds)},
		{"六張", hand(Three, Clubs, Four, Hearts, Five, Spades, Six, Clubs, Seven, Diamonds, Eight, Clubs)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewCombo(tt.cards); err == nil {
				t.Errorf("NewCombo() 應該出錯，但通過了")
			}
		})
	}
}

// TestStraightOrder 驗證順子的十個順位由小到大排列正確，
// 特別是 A2345 最小、23456 最大這兩個特例。
func TestStraightOrder(t *testing.T) {
	// 全部用梅花以外的固定花色組出來，確保比較的是順位而非花色。
	ascending := [][]Card{
		hand(Ace, Clubs, Two, Clubs, Three, Clubs, Four, Clubs, Five, Diamonds),
		hand(Three, Clubs, Four, Clubs, Five, Clubs, Six, Clubs, Seven, Diamonds),
		hand(Four, Clubs, Five, Clubs, Six, Clubs, Seven, Clubs, Eight, Diamonds),
		hand(Five, Clubs, Six, Clubs, Seven, Clubs, Eight, Clubs, Nine, Diamonds),
		hand(Six, Clubs, Seven, Clubs, Eight, Clubs, Nine, Clubs, Ten, Diamonds),
		hand(Seven, Clubs, Eight, Clubs, Nine, Clubs, Ten, Clubs, Jack, Diamonds),
		hand(Eight, Clubs, Nine, Clubs, Ten, Clubs, Jack, Clubs, Queen, Diamonds),
		hand(Nine, Clubs, Ten, Clubs, Jack, Clubs, Queen, Clubs, King, Diamonds),
		hand(Ten, Clubs, Jack, Clubs, Queen, Clubs, King, Clubs, Ace, Diamonds),
		hand(Two, Clubs, Three, Clubs, Four, Clubs, Five, Clubs, Six, Diamonds),
	}
	for i := 1; i < len(ascending); i++ {
		lo, hi := mustCombo(t, ascending[i-1]), mustCombo(t, ascending[i])
		if !hi.Beats(lo) {
			t.Errorf("順位 %d 應該壓過順位 %d", i, i-1)
		}
		if lo.Beats(hi) {
			t.Errorf("順位 %d 不該壓過順位 %d", i-1, i)
		}
	}
}

func TestComboBeats(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []Card
		wantBeat bool
	}{
		// 單張與對子
		{"2 壓 A", hand(Two, Clubs), hand(Ace, Spades), true},
		{"同點數比花色，黑桃最大", hand(Five, Spades), hand(Five, Hearts), true},
		{"梅花壓不過方塊", hand(Five, Clubs), hand(Five, Diamonds), false},
		{"大對子壓小對子", hand(King, Clubs, King, Hearts), hand(Queen, Spades, Queen, Hearts), true},
		{"同點數對子比較大那張", hand(Eight, Hearts, Eight, Spades), hand(Eight, Clubs, Eight, Diamonds), true},

		// 普通牌型不能跨牌型互壓
		{"對子壓不過單張", hand(Two, Spades, Two, Hearts), hand(Three, Clubs), false},
		{"單張壓不過對子", hand(Two, Spades), hand(Three, Clubs, Three, Hearts), false},
		{"葫蘆壓不過順子", hand(Ace, Clubs, Ace, Hearts, Ace, Spades, King, Clubs, King, Hearts),
			hand(Three, Clubs, Four, Hearts, Five, Spades, Six, Clubs, Seven, Diamonds), false},
		{"順子壓不過葫蘆", hand(Ten, Clubs, Jack, Hearts, Queen, Spades, King, Clubs, Ace, Diamonds),
			hand(Three, Clubs, Three, Hearts, Three, Spades, Four, Clubs, Four, Hearts), false},

		// 特殊牌型無條件壓普通牌型
		{"鐵支壓單張", hand(Three, Clubs, Three, Diamonds, Three, Hearts, Three, Spades, Four, Clubs),
			hand(Two, Spades), true},
		{"鐵支壓對子", hand(Three, Clubs, Three, Diamonds, Three, Hearts, Three, Spades, Four, Clubs),
			hand(Two, Spades, Two, Hearts), true},
		{"鐵支壓順子", hand(Three, Clubs, Three, Diamonds, Three, Hearts, Three, Spades, Four, Clubs),
			hand(Two, Clubs, Three, Hearts, Four, Spades, Five, Clubs, Six, Diamonds), true},
		{"鐵支壓葫蘆", hand(Three, Clubs, Three, Diamonds, Three, Hearts, Three, Spades, Four, Clubs),
			hand(Ace, Clubs, Ace, Hearts, Ace, Spades, King, Clubs, King, Hearts), true},
		{"同花順壓單張", hand(Ace, Spades, Two, Spades, Three, Spades, Four, Spades, Five, Spades),
			hand(Two, Spades), true},
		{"同花順壓葫蘆", hand(Ace, Spades, Two, Spades, Three, Spades, Four, Spades, Five, Spades),
			hand(Ace, Clubs, Ace, Hearts, Ace, Spades, King, Clubs, King, Hearts), true},
		{"最小同花順壓最大鐵支",
			hand(Ace, Spades, Two, Spades, Three, Spades, Four, Spades, Five, Spades),
			hand(Two, Clubs, Two, Diamonds, Two, Hearts, Two, Spades, Three, Clubs), true},
		{"鐵支壓不過同花順",
			hand(Two, Clubs, Two, Diamonds, Two, Hearts, Two, Spades, Three, Clubs),
			hand(Ace, Spades, Two, Spades, Three, Spades, Four, Spades, Five, Spades), false},
		{"普通牌型壓不過鐵支",
			hand(Ace, Clubs, Ace, Hearts, Ace, Spades, King, Clubs, King, Hearts),
			hand(Three, Clubs, Three, Diamonds, Three, Hearts, Three, Spades, Four, Clubs), false},

		// 同牌型比較
		{"葫蘆只比三條那組",
			hand(Eight, Clubs, Eight, Hearts, Eight, Spades, Three, Clubs, Three, Hearts),
			hand(Seven, Clubs, Seven, Hearts, Seven, Spades, Ace, Clubs, Ace, Hearts), true},
		{"鐵支只比四條那組",
			hand(Eight, Clubs, Eight, Diamonds, Eight, Hearts, Eight, Spades, Three, Clubs),
			hand(Seven, Clubs, Seven, Diamonds, Seven, Hearts, Seven, Spades, Ace, Clubs), true},
		{"同順位順子比比較牌的花色",
			hand(Six, Clubs, Seven, Clubs, Eight, Clubs, Nine, Clubs, Ten, Spades),
			hand(Six, Diamonds, Seven, Diamonds, Eight, Diamonds, Nine, Diamonds, Ten, Hearts), true},
		{"23456 用 2 當比較牌",
			hand(Two, Spades, Three, Clubs, Four, Clubs, Five, Clubs, Six, Clubs),
			hand(Two, Clubs, Three, Hearts, Four, Hearts, Five, Hearts, Six, Spades), true},
		{"同花順比順位",
			hand(Two, Hearts, Three, Hearts, Four, Hearts, Five, Hearts, Six, Hearts),
			hand(Ace, Spades, Two, Spades, Three, Spades, Four, Spades, Five, Spades), true},
		{"同順位同花順比花色",
			hand(Six, Spades, Seven, Spades, Eight, Spades, Nine, Spades, Ten, Spades),
			hand(Six, Hearts, Seven, Hearts, Eight, Hearts, Nine, Hearts, Ten, Hearts), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := mustCombo(t, tt.a), mustCombo(t, tt.b)
			if got := a.Beats(b); got != tt.wantBeat {
				t.Errorf("%v.Beats(%v) = %v, 想要 %v", a.Type, b.Type, got, tt.wantBeat)
			}
			// 大小關係必須反對稱：能互壓代表比較邏輯有錯。
			if tt.wantBeat && b.Beats(a) {
				t.Errorf("%v 與 %v 互相壓過對方", a.Type, b.Type)
			}
		})
	}
}
