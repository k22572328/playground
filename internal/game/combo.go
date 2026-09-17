package game

import (
	"errors"
	"slices"
)

// ComboType 牌型。只有六種合法牌型；三條與普通同花都不能單獨出牌。
type ComboType int

const (
	Invalid       ComboType = iota
	Single                  // 單張
	Pair                    // 對子
	Straight                // 順子
	FullHouse               // 葫蘆
	FourOfAKind             // 鐵支
	StraightFlush           // 同花順
)

var comboNames = map[ComboType]string{
	Invalid:       "無效牌型",
	Single:        "單張",
	Pair:          "對子",
	Straight:      "順子",
	FullHouse:     "葫蘆",
	FourOfAKind:   "鐵支",
	StraightFlush: "同花順",
}

func (t ComboType) String() string { return comboNames[t] }

// special 回報這是否為特殊牌型。特殊牌型（鐵支、同花順）可以無條件壓過
// 任何普通牌型，普通牌型之間則只能同牌型互壓。
func (t ComboType) special() bool { return t >= FourOfAKind }

var ErrInvalidCombo = errors.New("不是合法的牌型")

// Combo 一組已驗證過的出牌。
type Combo struct {
	Type  ComboType
	Cards []Card

	// key 是同牌型比大小時的關鍵牌：單張是自己，對子取較大那張，
	// 葫蘆與鐵支取相同點數那組的最大張，順子與同花順取比較牌。
	key Card

	// seq 是順子順位，僅順子與同花順使用。台灣大老二的順子順序是
	// A2345 最小、23456 最大，不是單純比最大點數，所以另外編號。
	seq int
}

// NewCombo 驗證 cards 是否構成合法牌型，並算出比大小需要的資訊。
// 回傳的 Combo 持有自己的一份牌，呼叫端之後改動 cards 不會影響它。
func NewCombo(cards []Card) (Combo, error) {
	sorted := make([]Card, len(cards))
	copy(sorted, cards)
	return newComboSorted(sorted)
}

// NewComboInto 與 NewCombo 相同，但把排序後的牌寫進 scratch 重複使用，
// 不另外配置記憶體。回傳的 Combo 會指向 scratch，所以下次呼叫就會被覆蓋：
// 只適合用來大量篩選候選牌組，要保留結果請改用 NewCombo 或 Clone。
// scratch 的長度必須與 cards 相同。
func NewComboInto(scratch, cards []Card) (Combo, error) {
	copy(scratch, cards)
	return newComboSorted(scratch)
}

// Clone 回傳一份自己持有牌的複本，讓從 NewComboInto 篩出來的結果能安全留存。
func (c Combo) Clone() Combo {
	cards := make([]Card, len(c.Cards))
	copy(cards, c.Cards)
	c.Cards = cards
	return c
}

// newComboSorted 與 NewCombo 相同，但直接沿用（並就地排序）傳入的 slice，
// 不再複製。呼叫端必須交出這份 slice 的所有權。
func newComboSorted(sorted []Card) (Combo, error) {
	slices.SortFunc(sorted, func(a, b Card) int { return a.order() - b.order() })

	if hasDuplicate(sorted) {
		return Combo{}, ErrInvalidCombo
	}

	switch len(sorted) {
	case 1:
		return Combo{Type: Single, Cards: sorted, key: sorted[0]}, nil
	case 2:
		if sorted[0].Rank != sorted[1].Rank {
			return Combo{}, ErrInvalidCombo
		}
		// 對子同點數時比較大的那張，排序後即最後一張。
		return Combo{Type: Pair, Cards: sorted, key: sorted[1]}, nil
	case 5:
		return newFiveCardCombo(sorted)
	default:
		return Combo{}, ErrInvalidCombo
	}
}

// newFiveCardCombo 判斷已排序的五張牌構成哪種牌型。
func newFiveCardCombo(sorted []Card) (Combo, error) {
	if seq, key, ok := asStraight(sorted); ok {
		t := Straight
		if sameSuit(sorted) {
			t = StraightFlush
		}
		return Combo{Type: t, Cards: sorted, key: key, seq: seq}, nil
	}

	// 鐵支與葫蘆都由點數的分組形狀決定，關鍵牌取數量較多那組的最大張。
	// 普通同花不是合法牌型，所以這裡不再判斷花色。
	size, last, ok := majorGroup(sorted)
	if !ok {
		return Combo{}, ErrInvalidCombo
	}
	switch size {
	case 4:
		return Combo{Type: FourOfAKind, Cards: sorted, key: sorted[last]}, nil
	case 3:
		return Combo{Type: FullHouse, Cards: sorted, key: sorted[last]}, nil
	}
	return Combo{}, ErrInvalidCombo
}

// majorGroup 檢查已排序的五張牌是否剛好分成兩種點數，
// 回傳張數較多那組的張數，以及該組最大張在 sorted 中的位置。
// 不是 4+1 或 3+2 的形狀就回傳 ok=false。
func majorGroup(sorted []Card) (size, last int, ok bool) {
	// 只有兩種點數時，第一組必定從 0 開始，第二組從分界點開始。
	split := -1
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Rank == sorted[i-1].Rank {
			continue
		}
		if split >= 0 {
			return 0, 0, false // 超過兩種點數
		}
		split = i
	}
	if split < 0 {
		return 0, 0, false // 五張同點數，不可能發生
	}

	first, second := split, len(sorted)-split
	switch {
	case first == 4 && second == 1, first == 3 && second == 2:
		// 較多的那組在前面，最大張是它的最後一張。
		return first, split - 1, true
	case second == 4 && first == 1, second == 3 && first == 2:
		// 較多的那組在後面，最大張是整手的最後一張。
		return second, len(sorted) - 1, true
	}
	return 0, 0, false
}

// straightSeqs 依照台灣大老二的順子順位，由小到大列出全部十種合法順子，
// 每種以組成的點數表示。A2345 最小、23456 最大，JQKA2 不成立。
var straightSeqs = [][5]Rank{
	{Ace, Two, Three, Four, Five},
	{Three, Four, Five, Six, Seven},
	{Four, Five, Six, Seven, Eight},
	{Five, Six, Seven, Eight, Nine},
	{Six, Seven, Eight, Nine, Ten},
	{Seven, Eight, Nine, Ten, Jack},
	{Eight, Nine, Ten, Jack, Queen},
	{Nine, Ten, Jack, Queen, King},
	{Ten, Jack, Queen, King, Ace},
	{Two, Three, Four, Five, Six},
}

// straightKeyRank 是各順子用來比大小的那張牌的點數。一般順子用最後一張，
// 但 23456 改用其中的 2，A2345 則用 5（A 與 2 都不是它的最大張）。
var straightKeyRank = [...]Rank{Five, Seven, Eight, Nine, Ten, Jack, Queen, King, Ace, Two}

// straightBySortedRanks 讓 asStraight 用一次查表就判斷出順位，
// 鍵是由小到大排好的五個點數。在 init 由 straightSeqs 建出來。
var straightBySortedRanks map[[5]Rank]int

func init() {
	straightBySortedRanks = make(map[[5]Rank]int, len(straightSeqs))
	for i, seq := range straightSeqs {
		key := seq
		slices.Sort(key[:])
		straightBySortedRanks[key] = i
	}
}

// asStraight 判斷是否為順子，回傳順位、比較牌與是否成立。
// sorted 必須已由小到大排好。
func asStraight(sorted []Card) (seq int, key Card, ok bool) {
	// sorted 依 order() 排序，點數本身也就跟著由小到大，可以直接取用。
	var ranks [5]Rank
	for i, c := range sorted {
		ranks[i] = c.Rank
	}

	seq, ok = straightBySortedRanks[ranks]
	if !ok {
		return 0, Card{}, false
	}
	// 比較牌是這個順位指定點數的那張牌。
	for _, c := range sorted {
		if c.Rank == straightKeyRank[seq] {
			return seq, c, true
		}
	}
	return 0, Card{}, false
}

func sameSuit(sorted []Card) bool {
	for _, c := range sorted[1:] {
		if c.Suit != sorted[0].Suit {
			return false
		}
	}
	return true
}

func hasDuplicate(sorted []Card) bool {
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == sorted[i-1] {
			return true
		}
	}
	return false
}

// Beats 回報 c 是否大得過 other。
//
// 普通牌型之間沒有跨牌型的大小關係，只能同牌型互壓；鐵支與同花順則可以
// 無條件壓過任何普通牌型，同花順又壓過任何鐵支。
func (c Combo) Beats(other Combo) bool {
	if c.Type != other.Type {
		// 只有特殊牌型能跨牌型壓制，且必須比對方強。
		return c.Type.special() && c.Type > other.Type
	}
	// 順子與同花順先比順位，同順位才比比較牌。
	if c.seq != other.seq {
		return c.seq > other.seq
	}
	return other.key.Less(c.key)
}

// Contains 回報這組牌是否包含指定的一張牌。
func (c Combo) Contains(card Card) bool { return slices.Contains(c.Cards, card) }
