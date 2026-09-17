package rules

import "fmt"

// Suit 花色。台灣大老二比大小：黑桃 > 紅心 > 方塊 > 梅花。
type Suit int

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

var suitNames = [...]string{"♣", "♦", "♥", "♠"}

func (s Suit) String() string { return suitNames[s] }

// Rank 點數。台灣大老二比大小：2 最大，接著 A、K…，3 最小。
type Rank int

const (
	Three Rank = iota
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
	Two
)

var rankNames = [...]string{"3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A", "2"}

func (r Rank) String() string { return rankNames[r] }

// Card 一張牌。零值是梅花 3，也就是開局必須先出的那張牌。
type Card struct {
	Rank Rank
	Suit Suit
}

func (c Card) String() string { return fmt.Sprintf("%s%s", c.Rank, c.Suit) }

// order 給出這張牌在單張比較下的絕對序位，愈大愈強。
func (c Card) order() int { return int(c.Rank)*4 + int(c.Suit) }

// Less 回報 c 是否小於 other，先比點數再比花色。
func (c Card) Less(other Card) bool { return c.order() < other.order() }

// ClubThree 是開局持有者必須先出的牌。
var ClubThree = Card{Rank: Three, Suit: Clubs}

// NewDeck 回傳一副排好序的 52 張牌。
func NewDeck() []Card {
	deck := make([]Card, 0, 52)
	for r := Three; r <= Two; r++ {
		for s := Clubs; s <= Spades; s++ {
			deck = append(deck, Card{Rank: r, Suit: s})
		}
	}
	return deck
}
