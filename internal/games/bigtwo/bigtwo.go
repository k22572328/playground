// Package bigtwo 把大老二接上平台的遊戲介面。
//
// 規則本身在 rules 與 match 兩個子套件裡，它們完全不知道平台的存在。
// 這一層負責翻譯：把平台傳來的動作解析成出牌，把牌局狀態整理成
// 前端看得懂的樣子。
package bigtwo

import (
	"encoding/json"
	"errors"

	"playground/internal/games/bigtwo/gamelog"
	"playground/internal/games/bigtwo/match"
	"playground/internal/games/bigtwo/rules"
	"playground/internal/platform"
)

// ID 是這個遊戲的識別碼，前端用它決定載入哪個牌桌畫面。
const ID = "bigtwo"

// Kind 描述大老二這個遊戲，供平台註冊。
func Kind() platform.Kind {
	return platform.Kind{
		ID:       ID,
		Name:     "大老二",
		MinSeats: match.NumPlayers, // 大老二固定四人
		MaxSeats: match.NumPlayers,
		New:      newInstance,
		NewRecorder: func(dir, roomID string, names []string) (platform.Recorder, error) {
			return gamelog.Create(dir, roomID, names)
		},
	}
}

// instance 是一局進行中的大老二。
type instance struct {
	m *match.Match
}

// newInstance 開一局。names 的長度由平台保證是四人。
func newInstance(names []string, s platform.Shuffler, rec platform.Recorder) platform.Instance {
	var seats [match.NumPlayers]string
	copy(seats[:], names)

	// 紀錄檔是選用的，而且平台只知道它能被關閉；
	// 實際寫入格式是大老二自己的事，所以在這裡轉回具體型別。
	var obs match.Observer
	if r, ok := rec.(*gamelog.Recorder); ok && r != nil {
		obs = r
	}
	return &instance{m: match.NewWithObserver(seats, s, obs)}
}

// action 是前端送來的一次出牌。cards 為空表示 PASS。
type action struct {
	Cards []cardRef `json:"cards"`
}

// cardRef 前端指定一張牌的方式，用點數與花色的數值表示。
type cardRef struct {
	Rank int `json:"rank"`
	Suit int `json:"suit"`
}

var errBadAction = errors.New("看不懂的出牌內容")

// Act 解析並執行一次出牌。
func (in *instance) Act(seat int, raw json.RawMessage) error {
	var a action
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return errBadAction
		}
	}

	cards, err := toCards(a.Cards)
	if err != nil {
		return err
	}
	return in.m.Play(seat, cards)
}

// ViewFor 整理出 seat 這個座位看得到的牌局狀態。
func (in *instance) ViewFor(seat int, offline []platform.Offline) any {
	v := newMatchView(in.m, seat)
	markOffline(v, offline)
	return v
}

// Over 回報這局是否結束。
func (in *instance) Over() bool { return in.m.Over() }

// toCards 把前端送來的牌轉成引擎用的型別，順便擋掉超出範圍的數值。
func toCards(refs []cardRef) ([]rules.Card, error) {
	if len(refs) == 0 {
		return nil, nil // PASS
	}
	cards := make([]rules.Card, 0, len(refs))
	for _, ref := range refs {
		if ref.Rank < int(rules.Three) || ref.Rank > int(rules.Two) {
			return nil, rules.ErrInvalidCombo
		}
		if ref.Suit < int(rules.Clubs) || ref.Suit > int(rules.Spades) {
			return nil, rules.ErrInvalidCombo
		}
		cards = append(cards, rules.Card{Rank: rules.Rank(ref.Rank), Suit: rules.Suit(ref.Suit)})
	}
	return cards, nil
}
