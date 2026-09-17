package match

import (
	"testing"

	"bigTwo/internal/game"
)

// rigged 是不洗牌的 Shuffler，讓測試拿到固定的牌局。
type rigged struct{}

func (rigged) Shuffle(int, func(i, j int)) {}

// newRigged 開一局用未洗牌牌堆發的遊戲。game.NewDeck 由小到大排列，
// 所以座位 0 拿到最小的 13 張，含梅花 3，因此座位 0 先出。
func newRigged(t *testing.T) *Match {
	t.Helper()
	m := New([NumPlayers]string{"A", "B", "C", "D"}, rigged{})
	if m.Turn != 0 {
		t.Fatalf("持梅花 3 的座位 0 應該先出，實際輪到 %d", m.Turn)
	}
	return m
}

// card 簡寫一張牌。
func card(r game.Rank, s game.Suit) game.Card { return game.Card{Rank: r, Suit: s} }

// setHands 直接指定每位玩家的手牌，用來擺出特定局面。
func setHands(m *Match, hands [NumPlayers][]game.Card) {
	for seat, h := range hands {
		m.Players[seat].Hand = h
	}
}

// mustPlay 打出一手牌，失敗即中止測試。
func mustPlay(t *testing.T, m *Match, seat int, cards []game.Card) {
	t.Helper()
	if err := m.Play(seat, cards); err != nil {
		t.Fatalf("座位 %d 出 %v 失敗: %v", seat, cards, err)
	}
}

// openWithClubThree 讓座位 0 用梅花 3 開局，之後就進入一般的 Round 流程。
// 回傳時檯面上是那張梅花 3，輪到座位 1。
func openWithClubThree(t *testing.T, m *Match) {
	t.Helper()
	mustPlay(t, m, 0, []game.Card{game.ClubThree})
}

func TestOpeningMustContainClubThree(t *testing.T) {
	m := newRigged(t)
	if err := m.Play(0, []game.Card{card(game.Three, game.Diamonds)}); err != ErrMustPlayClub3 {
		t.Errorf("出不含梅花 3 的第一手，err = %v, 想要 %v", err, ErrMustPlayClub3)
	}
	if err := m.Play(0, []game.Card{game.ClubThree}); err != nil {
		t.Errorf("出梅花 3 應該成功，err = %v", err)
	}
	if m.Turn != 1 {
		t.Errorf("出牌後應輪到座位 1，實際 %d", m.Turn)
	}
}

func TestNotYourTurn(t *testing.T) {
	m := newRigged(t)
	if err := m.Play(1, []game.Card{card(game.Three, game.Diamonds)}); err != ErrNotYourTurn {
		t.Errorf("err = %v, 想要 %v", err, ErrNotYourTurn)
	}
}

func TestCannotPassOnFreeLead(t *testing.T) {
	m := newRigged(t)
	if err := m.Play(0, nil); err != ErrCannotPass {
		t.Errorf("首攻 PASS 應被擋下，err = %v, 想要 %v", err, ErrCannotPass)
	}
}

func TestCannotPlayCardsYouDontHold(t *testing.T) {
	m := newRigged(t)
	openWithClubThree(t, m)
	// 黑桃 2 在座位 3 手上，座位 1 不能拿來出。
	if err := m.Play(1, []game.Card{card(game.Two, game.Spades)}); err != ErrNotYourCards {
		t.Errorf("err = %v, 想要 %v", err, ErrNotYourCards)
	}
}

// TestSuitBreaksTie 驗證同點數時由花色決勝：方塊 3 壓得過梅花 3。
func TestSuitBreaksTie(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]game.Card{
		0: {game.ClubThree, card(game.Nine, game.Clubs)},
		1: {card(game.Three, game.Diamonds), card(game.Nine, game.Diamonds)},
		2: {card(game.Four, game.Clubs), card(game.Ten, game.Clubs)},
		3: {card(game.Five, game.Clubs), card(game.Jack, game.Clubs)},
	})
	openWithClubThree(t, m)
	mustPlay(t, m, 1, []game.Card{card(game.Three, game.Diamonds)})
	mustPlay(t, m, 2, []game.Card{card(game.Four, game.Clubs)})
}

func TestTooSmallRejected(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]game.Card{
		0: {game.ClubThree, card(game.Nine, game.Clubs)},
		1: {card(game.Ace, game.Spades), card(game.Four, game.Clubs)},
		2: {card(game.Five, game.Clubs), card(game.Six, game.Clubs)},
		3: {card(game.Seven, game.Clubs), card(game.Eight, game.Clubs)},
	})
	openWithClubThree(t, m)
	// 檯面是梅花 3，座位 1 想用更小的牌壓 —— 手上沒有比 3 小的，
	// 改測同點數但花色更小的情形：先讓座位 1 出黑桃 A 拉高檯面。
	mustPlay(t, m, 1, []game.Card{card(game.Ace, game.Spades)})
	// 檯面是黑桃 A，座位 2 的梅花 5 壓不過。
	if err := m.Play(2, []game.Card{card(game.Five, game.Clubs)}); err != ErrTooSmall {
		t.Errorf("梅花 5 不該壓得過黑桃 A，err = %v, 想要 %v", err, ErrTooSmall)
	}
}

// TestWrongComboSizeRejected 驗證普通牌型之間不能跨牌型互壓：
// 檯面是單張時，出對子不算合法的壓制。
func TestWrongComboSizeRejected(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]game.Card{
		0: {game.ClubThree, card(game.Nine, game.Clubs)},
		1: {card(game.Four, game.Clubs), card(game.Four, game.Diamonds), card(game.Nine, game.Diamonds)},
		2: {card(game.Five, game.Clubs), card(game.Six, game.Clubs)},
		3: {card(game.Seven, game.Clubs), card(game.Eight, game.Clubs)},
	})
	openWithClubThree(t, m)
	if err := m.Play(1, []game.Card{card(game.Four, game.Clubs), card(game.Four, game.Diamonds)}); err != ErrTooSmall {
		t.Errorf("對子不該壓得過單張，err = %v, 想要 %v", err, ErrTooSmall)
	}
}

// TestPassForfeitsRound 驗證規格 13：玩家 PASS 之後，即使檯面被鐵支壓掉
// 而牌型改變，該玩家在本 Round 仍然不能再出牌。
func TestPassForfeitsRound(t *testing.T) {
	m := newRigged(t)
	// 每家都留有餘牌，確保這個 Round 中途沒有人離場，
	// 測到的才是 PASS 造成的資格喪失。
	setHands(m, [NumPlayers][]game.Card{
		0: {game.ClubThree, card(game.Eight, game.Clubs), card(game.Eight, game.Diamonds),
			card(game.Nine, game.Clubs)},
		// 座位 1 手握最大的鐵支，但 PASS 之後本 Round 就不能再用。
		1: {card(game.Two, game.Clubs), card(game.Two, game.Diamonds),
			card(game.Two, game.Hearts), card(game.Two, game.Spades), card(game.Four, game.Clubs)},
		2: {card(game.Jack, game.Clubs), card(game.Jack, game.Diamonds),
			card(game.Ten, game.Clubs)},
		3: {card(game.Three, game.Diamonds), card(game.Three, game.Hearts),
			card(game.Three, game.Spades), card(game.Five, game.Clubs), card(game.Six, game.Clubs)},
	})
	openWithClubThree(t, m)
	// 座位 1、2、3 都跟不上單張梅花 3 的 Round，全部 PASS，
	// 於是座位 0 取得下一個 Round 的首攻權。
	mustPlay(t, m, 1, nil)
	mustPlay(t, m, 2, nil)
	mustPlay(t, m, 3, nil)
	if m.Turn != 0 {
		t.Fatalf("三家 PASS 後應由座位 0 首攻，實際 %d", m.Turn)
	}
	if m.Table != nil {
		t.Fatal("Round 結束後檯面應該清空")
	}

	// 新 Round：座位 0 出對 8，座位 1 選擇 PASS（放棄本 Round 資格）。
	mustPlay(t, m, 0, []game.Card{card(game.Eight, game.Clubs), card(game.Eight, game.Diamonds)})
	mustPlay(t, m, 1, nil)
	// 座位 2 用對 J 壓過去，座位 3 跟不上也 PASS。
	mustPlay(t, m, 2, []game.Card{card(game.Jack, game.Clubs), card(game.Jack, game.Diamonds)})
	mustPlay(t, m, 3, nil)

	// 檯面持有者是座位 2，座位 1、3 都已 PASS，所以只剩座位 0 有資格。
	if m.Turn != 0 {
		t.Fatalf("座位 3 PASS 後應輪到仍有資格的座位 0，實際 %d（座位 1 已 PASS 不該再輪到）", m.Turn)
	}
	// 座位 1 已 PASS，即使手握鐵支也不能插隊出牌。
	if err := m.Play(1, []game.Card{
		card(game.Two, game.Clubs), card(game.Two, game.Diamonds),
		card(game.Two, game.Hearts), card(game.Two, game.Spades), card(game.Four, game.Clubs),
	}); err != ErrNotYourTurn {
		t.Errorf("已 PASS 的座位 1 不該能出牌，err = %v, 想要 %v", err, ErrNotYourTurn)
	}
}

// TestPassStateClearsNextRound 驗證規格 13：PASS 狀態只維持當前 Round，
// 進入下一個 Round 後所有人恢復出牌資格。
func TestPassStateClearsNextRound(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]game.Card{
		0: {game.ClubThree, card(game.Four, game.Clubs)},
		1: {card(game.Ace, game.Spades), card(game.Five, game.Clubs)},
		2: {card(game.Six, game.Clubs)},
		3: {card(game.Seven, game.Clubs)},
	})
	openWithClubThree(t, m)
	mustPlay(t, m, 1, nil) // 座位 1 在這個 Round PASS
	mustPlay(t, m, 2, nil)
	mustPlay(t, m, 3, nil)
	if m.Players[1].passed {
		t.Fatal("Round 結束後 PASS 狀態應清除")
	}
	// 新 Round 由座位 0 首攻，座位 1 應該恢復出牌資格。
	mustPlay(t, m, 0, []game.Card{card(game.Four, game.Clubs)})
	if m.Turn != 1 {
		t.Fatalf("新 Round 應輪到座位 1，實際 %d", m.Turn)
	}
	mustPlay(t, m, 1, []game.Card{card(game.Five, game.Clubs)})
}

// TestRankingAndSuccession 驗證規格 16：玩家打完最後一手就立刻取得名次，
// 且因為他已離場無法開下一個 Round，改由順位（下一個還有手牌的座位）接手。
func TestRankingAndSuccession(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]game.Card{
		0: {game.ClubThree}, // 打完這張就第一名
		1: {card(game.Four, game.Clubs), card(game.Nine, game.Clubs)},
		2: {card(game.Five, game.Clubs), card(game.Ten, game.Clubs)},
		3: {card(game.Six, game.Clubs), card(game.Jack, game.Clubs)},
	})
	openWithClubThree(t, m)

	// 名次在打完最後一手的當下就確定，不必等其他人 PASS。
	if m.Players[0].Rank != 1 {
		t.Fatalf("座位 0 打完手牌應得第一名，實際 %d", m.Players[0].Rank)
	}
	// 座位 0 已離場，Round 立刻結束，順位落到座位 1。
	if m.Turn != 1 {
		t.Errorf("順位應由座位 1 接手首攻，實際 %d", m.Turn)
	}
	if m.Table != nil {
		t.Error("新 Round 檯面應該是空的")
	}
	// 座位 1 是新 Round 首攻，可以自由出牌，也因此不能 PASS。
	if err := m.Play(1, nil); err != ErrCannotPass {
		t.Errorf("首攻不該能 PASS，err = %v", err)
	}
	mustPlay(t, m, 1, []game.Card{card(game.Nine, game.Clubs)})
}

// TestOutPlayersAreSkipped 驗證規格 17：已取得名次的玩家會被直接跳過，
// 不需要 PASS 也不參與 Round 結束判定。
func TestOutPlayersAreSkipped(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]game.Card{
		0: {game.ClubThree},
		1: {card(game.Four, game.Clubs), card(game.Nine, game.Clubs)},
		2: {card(game.Five, game.Clubs), card(game.Ten, game.Clubs)},
		3: {card(game.Six, game.Clubs), card(game.Jack, game.Clubs)},
	})
	openWithClubThree(t, m) // 座位 0 出完，第一名
	mustPlay(t, m, 1, []game.Card{card(game.Four, game.Clubs)})
	mustPlay(t, m, 2, []game.Card{card(game.Five, game.Clubs)})
	mustPlay(t, m, 3, []game.Card{card(game.Six, game.Clubs)})
	// 一圈回來時應該跳過已離場的座位 0，直接輪到座位 1。
	if m.Turn != 1 {
		t.Errorf("應跳過已離場的座位 0，實際輪到 %d", m.Turn)
	}
}

// TestFourthPlaceDecidedAutomatically 驗證規格 19：第三名產生時，
// 最後一位玩家直接判定為第四名，整局結束，不需把剩下的牌打完。
func TestFourthPlaceDecidedAutomatically(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]game.Card{
		0: {game.ClubThree},
		1: {card(game.Four, game.Clubs)},
		2: {card(game.Five, game.Clubs)},
		// 座位 3 還剩一堆牌，但不必打完。
		3: {card(game.Six, game.Clubs), card(game.Seven, game.Clubs), card(game.Eight, game.Clubs)},
	})
	openWithClubThree(t, m)                                     // 座位 0 第一名
	mustPlay(t, m, 1, []game.Card{card(game.Four, game.Clubs)}) // 座位 1 第二名
	mustPlay(t, m, 2, []game.Card{card(game.Five, game.Clubs)}) // 座位 2 第三名

	if !m.Over() {
		t.Fatal("第三名產生後整局應該結束")
	}
	if got := len(m.Players[3].Hand); got == 0 {
		t.Error("第四名不需要把牌打完，手牌不該是空的")
	}
	if err := m.Play(3, []game.Card{card(game.Six, game.Clubs)}); err != ErrMatchOver {
		t.Errorf("結束後還能出牌，err = %v, 想要 %v", err, ErrMatchOver)
	}

	ranked := m.Rankings()
	if len(ranked) != 3 {
		t.Fatalf("應該有 3 位玩家排出名次，實際 %d", len(ranked))
	}
	for i, want := range []int{0, 1, 2} {
		if ranked[i].Seat != want {
			t.Errorf("第 %d 名應該是座位 %d，實際 %d", i+1, want, ranked[i].Seat)
		}
	}
}

// TestLegalMovesAreAllPlayable 驗證 LegalMoves 回傳的每一組牌都真的打得出去。
func TestLegalMovesAreAllPlayable(t *testing.T) {
	m := newRigged(t)
	moves := m.LegalMoves(0)
	if len(moves) == 0 {
		t.Fatal("開局應該至少有一個合法出牌")
	}
	for _, mv := range moves {
		// 開局的每一組都必須含梅花 3。
		if !mv.Contains(game.ClubThree) {
			t.Errorf("開局的合法牌組 %v 應該包含梅花 3", mv.Cards)
		}
		if err := m.validate(0, mv); err != nil {
			t.Errorf("LegalMoves 回傳的 %v 卻無法通過驗證: %v", mv.Cards, err)
		}
	}
}
