package match

import (
	"testing"

	"playground/internal/games/bigtwo/rules"
)

// TestGoingOutEndsRoundImmediately 驗證打完最後一手就立刻結束 Round：
// 出完牌的人已經離場，沒有人需要再壓他的牌，所以檯面立刻清空，
// 順位者馬上取得自由出牌權，不必讓其他人再空跑一輪 PASS。
func TestGoingOutEndsRoundImmediately(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]rules.Card{
		0: {rules.ClubThree, card(rules.Eight, rules.Clubs), card(rules.Eight, rules.Diamonds)},
		// 其餘三家手上都是散牌，湊不出對子。
		1: {card(rules.Four, rules.Clubs), card(rules.Nine, rules.Clubs), card(rules.Ten, rules.Clubs)},
		2: {card(rules.Five, rules.Clubs), card(rules.Nine, rules.Diamonds), card(rules.Ten, rules.Diamonds)},
		3: {card(rules.Six, rules.Clubs), card(rules.Nine, rules.Hearts), card(rules.Ten, rules.Hearts)},
	})

	// 先讓座位 0 取得自由出牌權。
	openWithClubThree(t, m)
	mustPlay(t, m, 1, nil)
	mustPlay(t, m, 2, nil)
	mustPlay(t, m, 3, nil)

	// 座位 0 打出對 8，這是他的最後一手。
	mustPlay(t, m, 0, []rules.Card{card(rules.Eight, rules.Clubs), card(rules.Eight, rules.Diamonds)})

	// 名次當下就確定。
	if m.Players[0].Rank != 1 {
		t.Fatalf("座位 0 應立刻取得第一名，實際 %d", m.Players[0].Rank)
	}
	// Round 立刻結束，檯面清空。
	if m.Table != nil {
		t.Errorf("脫手後檯面應該清空，實際還有 %v", m.Table.Cards)
	}
	// 順位交給座位 1，而且他可以自由出任何合法牌型。
	if m.Turn != 1 {
		t.Fatalf("順位應交給座位 1，實際輪到 %d", m.Turn)
	}
	moves := m.LegalMoves(1)
	if len(moves) == 0 {
		t.Fatal("順位者應該能自由出牌，實際一組都不能出")
	}
	// 手上三張散牌，自由出牌時每張都能單獨打出去。
	if len(moves) != 3 {
		t.Errorf("座位 1 應有 3 種出法（三張單張），實際 %d 種", len(moves))
	}
}

// TestEveryoneElseOutEndsRound 驗證只剩一位對手時的 Round 結束判定：
// 其他人都已離場，該對手一 PASS，Round 就該立刻結束。
func TestEveryoneElseOutEndsRound(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]rules.Card{
		0: {rules.ClubThree},
		1: {card(rules.Four, rules.Clubs)},
		2: {card(rules.Five, rules.Clubs), card(rules.Ace, rules.Spades)},
		3: {card(rules.Six, rules.Clubs), card(rules.King, rules.Spades)},
	})
	openWithClubThree(t, m)                                        // 座位 0 出完 → 第一名
	mustPlay(t, m, 1, []rules.Card{card(rules.Four, rules.Clubs)}) // 座位 1 出完 → 第二名

	// 現在只剩座位 2、3 還有牌，檯面是梅花 4。
	mustPlay(t, m, 2, []rules.Card{card(rules.Five, rules.Clubs)})
	// 座位 3 一 PASS，就只剩座位 2 有資格 —— Round 立刻結束，由座位 2 首攻。
	mustPlay(t, m, 3, nil)
	if m.Turn != 2 {
		t.Errorf("Round 應由座位 2 贏下並首攻，實際輪到 %d", m.Turn)
	}
	if m.Table != nil {
		t.Error("Round 結束後檯面應清空")
	}
}

// TestSuccessionSkipsMultipleOutPlayers 驗證順位會連續跳過多位已離場的玩家。
func TestSuccessionSkipsMultipleOutPlayers(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]rules.Card{
		0: {rules.ClubThree},
		1: {card(rules.Four, rules.Clubs)},
		2: {card(rules.Five, rules.Clubs), card(rules.Nine, rules.Clubs)},
		3: {card(rules.Six, rules.Clubs), card(rules.Ten, rules.Clubs)},
	})
	openWithClubThree(t, m)                                        // 座位 0 離場
	mustPlay(t, m, 1, []rules.Card{card(rules.Four, rules.Clubs)}) // 座位 1 離場

	// 座位 2、3 打完這個 Round，最後由座位 3 贏下但他還有牌。
	mustPlay(t, m, 2, []rules.Card{card(rules.Five, rules.Clubs)})
	mustPlay(t, m, 3, []rules.Card{card(rules.Six, rules.Clubs)})
	// 繞回來時座位 0、1 都已離場，應直接跳到座位 2。
	if m.Turn != 2 {
		t.Errorf("應跳過已離場的座位 0、1，實際輪到 %d", m.Turn)
	}
}

// TestWinnerOfRoundLeavesSuccessionWraps 驗證順位會正確繞回座位 0。
// 座位 3 打完最後一手並贏下 Round，下一個首攻應繞回還有牌的座位 0。
func TestWinnerOfRoundLeavesSuccessionWraps(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]rules.Card{
		0: {rules.ClubThree, card(rules.Nine, rules.Clubs), card(rules.Nine, rules.Diamonds)},
		1: {card(rules.Four, rules.Clubs), card(rules.Ten, rules.Clubs)},
		2: {card(rules.Five, rules.Clubs), card(rules.Jack, rules.Clubs)},
		3: {card(rules.Six, rules.Clubs)}, // 打完就離場
	})
	openWithClubThree(t, m)
	mustPlay(t, m, 1, []rules.Card{card(rules.Four, rules.Clubs)})
	mustPlay(t, m, 2, []rules.Card{card(rules.Five, rules.Clubs)})
	mustPlay(t, m, 3, []rules.Card{card(rules.Six, rules.Clubs)}) // 座位 3 出完並領先

	if m.Players[3].Rank != 1 {
		t.Fatalf("座位 3 應得第一名，實際 %d", m.Players[3].Rank)
	}
	// 座位 3 一脫手 Round 就結束，不必等其他人 PASS。
	// 順位從座位 3 的下一位開始找，也就是繞回座位 0。
	if m.Turn != 0 {
		t.Errorf("順位應繞回座位 0，實際 %d", m.Turn)
	}
	if m.Table != nil {
		t.Error("脫手後檯面應該清空")
	}
	// 座位 0 是新 Round 的首攻，可以自由出牌。
	mustPlay(t, m, 0, []rules.Card{card(rules.Nine, rules.Clubs), card(rules.Nine, rules.Diamonds)})
}

// TestSpecialComboBreaksIntoNormalRound 驗證規格第 5 條：
// 普通 Round 被鐵支壓掉後，後續只能出更大的鐵支或同花順，回不去原本的牌型。
// 這正是規格舉的例子：A 出 88、B 出 JJ、C 用 7777+3 炸掉，之後不能再出 QQ。
func TestSpecialComboBreaksIntoNormalRound(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]rules.Card{
		0: {rules.ClubThree, card(rules.Eight, rules.Clubs), card(rules.Eight, rules.Diamonds),
			card(rules.Four, rules.Clubs)},
		1: {card(rules.Jack, rules.Clubs), card(rules.Jack, rules.Diamonds),
			card(rules.Queen, rules.Clubs), card(rules.Queen, rules.Diamonds)},
		// 座位 2 手握鐵支，等著炸掉對子 Round。多留一張牌，
		// 免得打出鐵支就脫手，那樣 Round 會立刻結束，測不到被炸之後的限制。
		2: {card(rules.Seven, rules.Clubs), card(rules.Seven, rules.Diamonds),
			card(rules.Seven, rules.Hearts), card(rules.Seven, rules.Spades),
			card(rules.Nine, rules.Clubs), card(rules.Ten, rules.Clubs)},
		3: {card(rules.King, rules.Clubs), card(rules.King, rules.Diamonds),
			card(rules.Two, rules.Clubs), card(rules.Two, rules.Diamonds)},
	})

	// 先用梅花 3 開局，讓大家 PASS 掉，好讓座位 0 自由出對子。
	openWithClubThree(t, m)
	mustPlay(t, m, 1, nil)
	mustPlay(t, m, 2, nil)
	mustPlay(t, m, 3, nil)

	// 對子 Round 開始。
	mustPlay(t, m, 0, []rules.Card{card(rules.Eight, rules.Clubs), card(rules.Eight, rules.Diamonds)})
	mustPlay(t, m, 1, []rules.Card{card(rules.Jack, rules.Clubs), card(rules.Jack, rules.Diamonds)})

	// 座位 2 用鐵支炸掉對子 Round。
	quad := []rules.Card{
		card(rules.Seven, rules.Clubs), card(rules.Seven, rules.Diamonds),
		card(rules.Seven, rules.Hearts), card(rules.Seven, rules.Spades),
		card(rules.Nine, rules.Clubs),
	}
	mustPlay(t, m, 2, quad)
	if m.Table.Type != rules.FourOfAKind {
		t.Fatalf("檯面應該是鐵支，實際 %v", m.Table.Type)
	}

	// 輪到座位 3：手上有 KK 與 22 兩組對子，但檯面已經是鐵支，都不能出。
	for _, pair := range [][]rules.Card{
		{card(rules.King, rules.Clubs), card(rules.King, rules.Diamonds)},
		{card(rules.Two, rules.Clubs), card(rules.Two, rules.Diamonds)},
	} {
		if err := m.Play(3, pair); err != ErrTooSmall {
			t.Errorf("鐵支檯面上出對子 %v，err = %v, 想要 %v", pair, err, ErrTooSmall)
		}
	}

	// 座位 3 的合法出牌應該一組都沒有（他沒有更大的鐵支或同花順）。
	if moves := m.LegalMoves(3); len(moves) != 0 {
		t.Errorf("座位 3 不該有任何合法出牌，實際 %d 組", len(moves))
	}
	// 只能 PASS。
	mustPlay(t, m, 3, nil)
}

// TestCannotPlayAfterRanked 驗證已取得名次的玩家不能再出牌。
func TestCannotPlayAfterRanked(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]rules.Card{
		0: {rules.ClubThree},
		1: {card(rules.Four, rules.Clubs), card(rules.Nine, rules.Clubs)},
		2: {card(rules.Five, rules.Clubs), card(rules.Ten, rules.Clubs)},
		3: {card(rules.Six, rules.Clubs), card(rules.Jack, rules.Clubs)},
	})
	openWithClubThree(t, m) // 座位 0 出完離場

	// 座位 0 已離場，不論出什麼都該被擋 —— 現在也輪不到他。
	if err := m.Play(0, []rules.Card{card(rules.Nine, rules.Clubs)}); err != ErrNotYourTurn {
		t.Errorf("已離場的玩家出牌 err = %v, 想要 %v", err, ErrNotYourTurn)
	}
}

// TestLegalMovesEmptyWhenNotYourTurn 驗證不是自己的回合時拿不到任何合法出牌。
func TestLegalMovesEmptyWhenNotYourTurn(t *testing.T) {
	m := newRigged(t)
	for seat := 1; seat < NumPlayers; seat++ {
		if moves := m.LegalMoves(seat); moves != nil {
			t.Errorf("座位 %d 不是當前回合，不該有合法出牌，實際 %d 組", seat, len(moves))
		}
	}
}

// TestLegalMovesRespectsSpecialOverride 驗證檯面是單張時，
// LegalMoves 仍會把可以強壓的鐵支與同花順算進去。
func TestLegalMovesRespectsSpecialOverride(t *testing.T) {
	m := newRigged(t)
	setHands(m, [NumPlayers][]rules.Card{
		0: {rules.ClubThree, card(rules.Nine, rules.Clubs)},
		// 座位 1 手上有一副鐵支，應該能用來壓單張。
		1: {card(rules.Seven, rules.Clubs), card(rules.Seven, rules.Diamonds),
			card(rules.Seven, rules.Hearts), card(rules.Seven, rules.Spades),
			card(rules.Nine, rules.Diamonds)},
		2: {card(rules.Five, rules.Clubs), card(rules.Ten, rules.Clubs)},
		3: {card(rules.Six, rules.Clubs), card(rules.Jack, rules.Clubs)},
	})
	openWithClubThree(t, m)

	var foundQuad bool
	for _, mv := range m.LegalMoves(1) {
		if mv.Type == rules.FourOfAKind {
			foundQuad = true
		}
	}
	if !foundQuad {
		t.Error("檯面是單張時，鐵支也該是合法出牌（可以強壓）")
	}
}

// TestPlayEmptyHandRejected 驗證出一組不在手上的牌會被擋，而不是讓狀態出錯。
func TestPlayEmptyHandRejected(t *testing.T) {
	m := newRigged(t)
	openWithClubThree(t, m)

	// 座位 1 謊報一組自己沒有的鐵支。
	fake := []rules.Card{
		card(rules.Two, rules.Clubs), card(rules.Two, rules.Diamonds),
		card(rules.Two, rules.Hearts), card(rules.Two, rules.Spades),
		card(rules.Three, rules.Diamonds),
	}
	if err := m.Play(1, fake); err != ErrNotYourCards {
		t.Errorf("出沒有的牌 err = %v, 想要 %v", err, ErrNotYourCards)
	}
	// 狀態不該被破壞：仍輪到座位 1，手牌張數不變。
	if m.Turn != 1 {
		t.Errorf("失敗的出牌不該換手，實際輪到 %d", m.Turn)
	}
	if len(m.Players[1].Hand) != CardsPerHand {
		t.Errorf("失敗的出牌不該減少手牌，實際 %d 張", len(m.Players[1].Hand))
	}
}

// TestDealIsCompleteAndDisjoint 驗證發牌的正確性：52 張不重不漏，每人 13 張。
func TestDealIsCompleteAndDisjoint(t *testing.T) {
	m := newRigged(t)

	seen := make(map[rules.Card]int)
	for _, p := range m.Players {
		if len(p.Hand) != CardsPerHand {
			t.Errorf("座位 %d 拿到 %d 張，應該是 %d 張", p.Seat, len(p.Hand), CardsPerHand)
		}
		for _, c := range p.Hand {
			seen[c]++
		}
	}
	if len(seen) != 52 {
		t.Errorf("發出去 %d 種不同的牌，應該是 52 種", len(seen))
	}
	for c, n := range seen {
		if n != 1 {
			t.Errorf("%v 出現了 %d 次", c, n)
		}
	}
}

// TestHandsStaySorted 驗證發牌後手牌是排好序的，前端才能穩定顯示。
func TestHandsStaySorted(t *testing.T) {
	m := newRigged(t)
	for _, p := range m.Players {
		for i := 1; i < len(p.Hand); i++ {
			if !p.Hand[i-1].Less(p.Hand[i]) {
				t.Errorf("座位 %d 的手牌沒有排序: %v 出現在 %v 之前",
					p.Seat, p.Hand[i-1], p.Hand[i])
				break
			}
		}
	}
}
