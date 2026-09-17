package server

import (
	"bigTwo/internal/game"
	"bigTwo/internal/lobby"
	"bigTwo/internal/match"
)

// 這個檔案定義送給前端的資料形狀。關鍵原則：每位玩家只看得到自己的手牌，
// 別人的手牌一律只揭露張數。

// cardView 一張牌。用 rank/suit 的數值讓前端自行決定怎麼畫。
type cardView struct {
	Rank int    `json:"rank"`
	Suit int    `json:"suit"`
	Text string `json:"text"` // 例如 "3♣"，方便除錯與無障礙讀出
}

func newCardView(c game.Card) cardView {
	return cardView{Rank: int(c.Rank), Suit: int(c.Suit), Text: c.String()}
}

func newCardViews(cards []game.Card) []cardView {
	out := make([]cardView, len(cards))
	for i, c := range cards {
		out[i] = newCardView(c)
	}
	return out
}

// comboView 檯面上或紀錄裡的一組牌。
type comboView struct {
	Type  string     `json:"type"`
	Cards []cardView `json:"cards"`
}

func newComboView(c *game.Combo) *comboView {
	if c == nil {
		return nil
	}
	return &comboView{Type: c.Type.String(), Cards: newCardViews(c.Cards)}
}

// seatView 牌桌上的一位玩家，從某個視角看出去的樣子。
type seatView struct {
	Seat      int    `json:"seat"`
	Name      string `json:"name"`
	CardCount int    `json:"cardCount"`
	Rank      int    `json:"rank"` // 0 表示還在場上
	IsYou     bool   `json:"isYou"`
	IsTurn    bool   `json:"isTurn"`
	Passed    bool   `json:"passed"` // 本 Round 已 PASS，失去出牌資格
}

// matchView 一局遊戲在某位玩家眼中的完整狀態。
type matchView struct {
	Seats []seatView `json:"seats"`
	Table *comboView `json:"table"` // nil 表示自由出牌

	// YourSeat 是這位玩家的座位；-1 表示他是旁觀者。
	YourSeat int        `json:"yourSeat"`
	YourHand []cardView `json:"yourHand"`

	// CanPlay 表示現在輪到這位玩家，CanPass 表示他可以選擇 PASS。
	CanPlay bool `json:"canPlay"`
	CanPass bool `json:"canPass"`

	Over     bool       `json:"over"`
	Rankings []rankView `json:"rankings"`
}

// rankView 結算時的名次。
type rankView struct {
	Rank int    `json:"rank"`
	Seat int    `json:"seat"`
	Name string `json:"name"`
}

// newMatchView 把一局的狀態轉成 viewer 這個座位看得到的樣子。
// viewer 為 -1 代表旁觀者，看不到任何人的手牌。
func newMatchView(m *match.Match, viewer int) *matchView {
	if m == nil {
		return nil
	}

	v := &matchView{
		Table:    newComboView(m.Table),
		YourSeat: viewer,
		Over:     m.Over(),
		Seats:    make([]seatView, 0, match.NumPlayers),
	}

	for _, p := range m.Players {
		v.Seats = append(v.Seats, seatView{
			Seat:      p.Seat,
			Name:      p.Name,
			CardCount: len(p.Hand),
			Rank:      p.Rank,
			IsYou:     p.Seat == viewer,
			IsTurn:    !m.Over() && p.Seat == m.Turn,
			Passed:    m.HasPassed(p.Seat),
		})
	}

	if viewer >= 0 && viewer < match.NumPlayers {
		v.YourHand = newCardViews(m.Players[viewer].Hand)
		v.CanPlay = !m.Over() && m.Turn == viewer
		// 檯面上沒牌時是自由出牌，必須出牌不能 PASS。
		v.CanPass = v.CanPlay && m.Table != nil
	}

	for _, p := range m.Rankings() {
		v.Rankings = append(v.Rankings, rankView{Rank: p.Rank, Seat: p.Seat, Name: p.Name})
	}
	return v
}

// roomView 大廳列表或房間內看到的一個房間。
type roomView struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Seats   []slotView `json:"seats"`
	Started bool       `json:"started"`
	Full    bool       `json:"full"`

	// YouAreHost 讓前端決定要不要顯示踢人與開始按鈕。
	YouAreHost bool `json:"youAreHost"`
}

// slotView 房間裡的一個位子（尚未開始遊戲時）。
type slotView struct {
	// PlayerID 讓房長的踢人按鈕知道要踢誰。
	PlayerID string `json:"playerId"`
	Name     string `json:"name"`
	IsHost   bool   `json:"isHost"`
	IsYou    bool   `json:"isYou"`
}

func newRoomView(r *lobby.Room, viewerID string) roomView {
	v := roomView{
		ID:         r.ID,
		Name:       r.Name,
		Started:    r.Started,
		Full:       r.Full(),
		YouAreHost: r.IsHost(viewerID),
		Seats:      make([]slotView, 0, len(r.Seats)),
	}
	for i, s := range r.Seats {
		v.Seats = append(v.Seats, slotView{
			PlayerID: s.PlayerID,
			Name:     s.Name,
			IsHost:   i == 0, // 座位 0 永遠是房長
			IsYou:    s.PlayerID == viewerID,
		})
	}
	return v
}
