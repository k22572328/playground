// Package match 負責一局大老二的流程：發牌、Round 的開始與結束、
// PASS 資格、名次與順位。牌型的合法性與大小一律交給 game 判斷。
package match

import (
	"errors"
	"slices"
	"sort"

	"bigTwo/internal/game"
)

const (
	NumPlayers   = 4
	CardsPerHand = 13

	// none 表示「沒有這個玩家」，用於尚未決定的首攻或檯面持有者。
	none = -1
)

var (
	ErrNotYourTurn   = errors.New("還沒輪到你")
	ErrNotYourCards  = errors.New("你手上沒有這些牌")
	ErrMustPlayClub3 = errors.New("第一手必須包含梅花 3")
	ErrTooSmall      = errors.New("壓不過檯面上的牌")
	ErrCannotPass    = errors.New("你是這一輪的首攻，不能 PASS")
	ErrMatchOver     = errors.New("這局已經結束了")
)

// Player 一位玩家在這一局中的狀態。
type Player struct {
	Seat int
	Name string
	Hand []game.Card

	// Rank 是名次 1~4；0 表示還沒排出名次。
	Rank int

	// passed 表示在目前這個 Round 已經 PASS，失去本 Round 的出牌資格。
	// 每個 Round 開始時清除。
	passed bool
}

// Out 回報這位玩家是否已經打完手牌離場。
func (p *Player) Out() bool { return p.Rank != 0 }

// Match 一局大老二。
type Match struct {
	Players [NumPlayers]*Player

	// Turn 是目前該出牌的座位。
	Turn int

	// Table 是檯面上待壓的牌；nil 代表目前是自由出牌。
	Table *game.Combo

	// leader 是打出檯面這手牌的座位，Round 結束時由他取得下個 Round 首攻。
	leader int

	// nextRank 是下一位打完手牌的玩家會拿到的名次。
	nextRank int

	// Log 記錄這局的每一手，供前端顯示與重播。
	Log []Event

	// observer 在每次狀態變化時被呼叫，用來把牌局寫進紀錄檔。
	// 規則層只負責「說發生了什麼」，怎麼記錄是外層的事。
	observer Observer
}

// Event 一次出牌或 PASS 的紀錄。
type Event struct {
	Seat  int         `json:"seat"`
	Combo *game.Combo `json:"combo"` // nil 表示 PASS
	Rank  int         `json:"rank"`  // 非 0 表示這手打完後取得的名次
}

// Observer 接收一局之中的每個重要事件。所有方法都在 Match 的操作過程中
// 同步呼叫，實作者不應該阻塞太久，也不該回頭呼叫 Match。
type Observer interface {
	// Dealt 在發完牌時呼叫，hands 依座位順序給出每家的初始手牌。
	Dealt(hands [NumPlayers][]game.Card, first int)

	// Played 在成功出牌後呼叫。rank 非 0 表示這手打完後取得名次。
	Played(seat int, combo game.Combo, rank int)

	// Passed 在玩家 PASS 後呼叫。
	Passed(seat int)

	// RoundEnded 在一個 Round 結束時呼叫，starter 是下一個 Round 的首攻者。
	// bySuccession 為真表示贏下 Round 的人已離場，首攻權由順位遞補。
	RoundEnded(winner, starter int, bySuccession bool)

	// Finished 在整局結束時呼叫，ranks 依名次列出座位，最後一個是第四名。
	Finished(ranks []int)
}

// Shuffler 把牌洗亂。抽成介面是為了讓測試可以給定固定牌局。
type Shuffler interface {
	Shuffle(n int, swap func(i, j int))
}

// New 開一局新遊戲：洗牌、發牌，持有梅花 3 的玩家取得第一個出牌權。
func New(names [NumPlayers]string, s Shuffler) *Match {
	return NewWithObserver(names, s, nil)
}

// NewWithObserver 與 New 相同，但額外把每個事件通知 obs，用來寫牌局紀錄。
// obs 為 nil 時不做任何通知。
func NewWithObserver(names [NumPlayers]string, s Shuffler, obs Observer) *Match {
	deck := game.NewDeck()
	s.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	m := &Match{leader: none, nextRank: 1, observer: obs}
	var dealt [NumPlayers][]game.Card
	for seat := range m.Players {
		h := deck[seat*CardsPerHand : (seat+1)*CardsPerHand]
		sort.Slice(h, func(a, b int) bool { return h[a].Less(h[b]) })
		m.Players[seat] = &Player{Seat: seat, Name: names[seat], Hand: h}
		if slices.Contains(h, game.ClubThree) {
			m.Turn = seat
		}
		// 交給觀察者的是複本，之後出牌不會動到紀錄裡的初始手牌。
		dealt[seat] = append([]game.Card(nil), h...)
	}

	if obs != nil {
		obs.Dealt(dealt, m.Turn)
	}
	return m
}

// Over 回報整局是否結束。第三名產生時，最後一位玩家直接判定為第四名，
// 不需要把剩下的牌打完。
func (m *Match) Over() bool { return m.nextRank > NumPlayers-1 }

// HasPassed 回報 seat 是否已在目前這個 Round 放棄出牌資格。
func (m *Match) HasPassed(seat int) bool { return m.Players[seat].passed }

// Rankings 依名次由第一名排到第四名。
func (m *Match) Rankings() []*Player {
	ranked := make([]*Player, 0, NumPlayers)
	for _, p := range m.Players {
		if p.Out() {
			ranked = append(ranked, p)
		}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Rank < ranked[j].Rank })
	return ranked
}

// isOpening 回報是否還沒有人出過牌，也就是必須打出梅花 3 的那一手。
func (m *Match) isOpening() bool { return m.leader == none }

// Play 讓 seat 打出 cards；cards 為空代表 PASS。
func (m *Match) Play(seat int, cards []game.Card) error {
	if m.Over() {
		return ErrMatchOver
	}
	if seat != m.Turn {
		return ErrNotYourTurn
	}
	if len(cards) == 0 {
		return m.pass(seat)
	}

	combo, err := game.NewCombo(cards)
	if err != nil {
		return err
	}
	if err := m.validate(seat, combo); err != nil {
		return err
	}

	p := m.Players[seat]
	p.Hand = slices.DeleteFunc(p.Hand, func(c game.Card) bool { return combo.Contains(c) })
	m.Table = &combo
	m.leader = seat

	ev := Event{Seat: seat, Combo: &combo}
	out := len(p.Hand) == 0
	if out {
		// 名次在打完最後一手的當下就確定，不必等其他人 PASS。
		p.Rank = m.nextRank
		m.nextRank++
		ev.Rank = p.Rank
	}
	m.Log = append(m.Log, ev)
	if m.observer != nil {
		m.observer.Played(seat, combo, ev.Rank)
	}

	// 打完最後一手的人已經離場，這個 Round 沒有人需要再壓他的牌，
	// 所以直接結束 Round，由順位者重新自由出牌。
	if out {
		if m.Over() {
			m.notifyFinished()
		} else {
			m.endRound()
		}
		return nil
	}

	m.advance()
	return nil
}

// notifyFinished 在整局結束時通知觀察者。
func (m *Match) notifyFinished() {
	if m.observer != nil {
		m.observer.Finished(m.finalOrder())
	}
}

// validate 檢查這組牌在目前狀態下能不能打出去。
func (m *Match) validate(seat int, combo game.Combo) error {
	if !m.holdsAll(seat, combo.Cards) {
		return ErrNotYourCards
	}
	// 整局的第一手必須把梅花 3 打出去，之後就沒有這個限制。
	if m.isOpening() && !combo.Contains(game.ClubThree) {
		return ErrMustPlayClub3
	}
	if m.Table != nil && !combo.Beats(*m.Table) {
		return ErrTooSmall
	}
	return nil
}

// pass 記錄一次 PASS。PASS 之後該玩家在本 Round 永久失去出牌資格，
// 即使之後檯面被鐵支或同花順壓掉也不能再回來。
func (m *Match) pass(seat int) error {
	// 自由出牌時沒有東西要壓，代表你是本 Round 首攻，必須出牌。
	if m.Table == nil {
		return ErrCannotPass
	}
	m.Players[seat].passed = true
	m.Log = append(m.Log, Event{Seat: seat, Combo: nil})
	if m.observer != nil {
		m.observer.Passed(seat)
	}
	m.advance()
	return nil
}

// advance 把出牌權交給下一位有資格的玩家；若本 Round 已無人能壓，
// 則結束 Round 並決定下一個 Round 的首攻者。
func (m *Match) advance() {
	if m.Over() {
		m.notifyFinished()
		return
	}
	if next, ok := m.nextEligible(); ok {
		m.Turn = next
		return
	}
	m.endRound()
}

// finalOrder 依名次列出座位，最後補上唯一沒排上名次的那位（第四名）。
func (m *Match) finalOrder() []int {
	order := make([]int, 0, NumPlayers)
	for _, p := range m.Rankings() {
		order = append(order, p.Seat)
	}
	for _, p := range m.Players {
		if !p.Out() {
			order = append(order, p.Seat)
		}
	}
	return order
}

// nextEligible 從目前座位往下找還能在本 Round 出牌的玩家：
// 必須還有手牌、本 Round 沒 PASS 過，且不是檯面持有者本人。
func (m *Match) nextEligible() (int, bool) {
	for i := 1; i < NumPlayers; i++ {
		seat := (m.Turn + i) % NumPlayers
		if seat == m.leader {
			continue
		}
		if p := m.Players[seat]; !p.Out() && !p.passed {
			return seat, true
		}
	}
	return none, false
}

// endRound 結束目前 Round：清掉檯面與 PASS 狀態，並決定首攻者。
// 檯面持有者若已離場，改由順位接手 —— 從他的下一個座位開始，
// 找到第一位還有手牌的玩家。
func (m *Match) endRound() {
	m.Table = nil
	for _, p := range m.Players {
		p.passed = false
	}

	winner := m.leader
	starter := winner
	// 贏下 Round 的人若已離場，改由順位接手：他成為新 Round 的首攻者，
	// 檯面已清空，可以自由出任何合法牌型。
	bySuccession := m.Players[starter].Out()
	if bySuccession {
		for i := 1; i < NumPlayers; i++ {
			seat := (m.leader + i) % NumPlayers
			if !m.Players[seat].Out() {
				starter = seat
				break
			}
		}
	}
	m.Turn = starter
	m.leader = starter

	if m.observer != nil {
		m.observer.RoundEnded(winner, starter, bySuccession)
	}
}

// holdsAll 回報 seat 手上是否真的有這些牌。
func (m *Match) holdsAll(seat int, cards []game.Card) bool {
	for _, c := range cards {
		if !slices.Contains(m.Players[seat].Hand, c) {
			return false
		}
	}
	return true
}

// LegalMoves 列出 seat 現在所有打得出去的牌組，供提示與電腦玩家使用。
// 回傳的每一組都保證能通過 Play 的驗證。
func (m *Match) LegalMoves(seat int) []game.Combo {
	if m.Over() || seat != m.Turn {
		return nil
	}
	var moves []game.Combo
	scratch := make([]game.Card, 5) // 五張是最長的牌型，一個緩衝區夠用
	for _, size := range m.playableSizes() {
		m.eachCombination(m.Players[seat].Hand, size, func(cards []game.Card) {
			// 絕大多數候選都會被打回票，所以先用共用緩衝區判斷，
			// 確定留下來時才複製一份給 Combo 自己保管。
			combo, err := game.NewComboInto(scratch[:size], cards)
			if err != nil {
				return
			}
			if m.validate(seat, combo) != nil {
				return
			}
			moves = append(moves, combo.Clone())
		})
	}
	return moves
}

// playableSizes 給出目前局面下值得嘗試的出牌張數。檯面上有牌時，
// 只有同張數的牌型壓得過它，唯一的例外是鐵支與同花順這兩種五張的特殊牌型，
// 它們可以壓任何普通牌型，所以五張永遠要試。
func (m *Match) playableSizes() []int {
	if m.Table == nil {
		return []int{1, 2, 5}
	}
	switch n := len(m.Table.Cards); n {
	case 5:
		return []int{5}
	default:
		// 同張數的普通牌型，外加可以強壓的五張特殊牌型。
		return []int{n, 5}
	}
}

// eachCombination 依序把手牌中每個 size 張的組合交給 fn。
// 傳給 fn 的 slice 會被重複使用，需要保留內容的呼叫端必須自行複製；
// game.NewCombo 本身就會複製一份，所以這裡可以安全共用緩衝區。
func (m *Match) eachCombination(hand []game.Card, size int, fn func([]game.Card)) {
	if size > len(hand) {
		return
	}
	buf := make([]game.Card, size)
	var build func(start, depth int)
	build = func(start, depth int) {
		if depth == size {
			fn(buf)
			return
		}
		// 預留足夠的牌填滿剩下的位置，避免走進不可能湊齊的分支。
		for i := start; i <= len(hand)-(size-depth); i++ {
			buf[depth] = hand[i]
			build(i+1, depth+1)
		}
	}
	build(0, 0)
}
