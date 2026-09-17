package match

import (
	"math/rand"
	"testing"
)

// TestRandomMatchesAlwaysFinish 用隨機合法出牌跑完大量對局，確認流程不會卡死，
// 且每局都能正常排出前三名。這是規則層的整體性保險：任何讓 Round 無法結束
// 或輪次卡住的錯誤，都會在這裡被抓出來。
func TestRandomMatchesAlwaysFinish(t *testing.T) {
	names := [NumPlayers]string{"A", "B", "C", "D"}

	// 平常跑 2 千局就足以擋下大多數迴歸；完整的十萬局用 go test 不加 -short 執行。
	matches := 100_000
	if testing.Short() {
		matches = 2_000
	}

	for seed := range matches {
		r := rand.New(rand.NewSource(int64(seed)))
		m := New(names, r)

		// 一局最多 52 次出牌加上有限次 PASS，這個上限遠大於實際需要，
		// 超過就代表流程卡住了。
		const maxSteps = 2000
		steps := 0
		for !m.Over() {
			if steps++; steps > maxSteps {
				t.Fatalf("seed %d: 超過 %d 步仍未結束，流程可能卡住 (turn=%d leader=%d)",
					seed, maxSteps, m.Turn, m.leader)
			}

			seat := m.Turn
			if p := m.Players[seat]; p.Out() {
				t.Fatalf("seed %d: 輪到已離場的座位 %d", seed, seat)
			}

			moves := m.LegalMoves(seat)
			// 沒有牌能壓就只能 PASS；自由出牌時一定至少有一個合法選擇。
			if len(moves) == 0 {
				if m.Table == nil {
					t.Fatalf("seed %d: 座位 %d 自由出牌卻無合法牌組", seed, seat)
				}
				if err := m.Play(seat, nil); err != nil {
					t.Fatalf("seed %d: 座位 %d PASS 失敗: %v", seed, seat, err)
				}
				continue
			}
			pick := moves[r.Intn(len(moves))]
			if err := m.Play(seat, pick.Cards); err != nil {
				t.Fatalf("seed %d: 座位 %d 出 LegalMoves 給的 %v 卻失敗: %v",
					seed, seat, pick.Cards, err)
			}
		}

		ranked := m.Rankings()
		if len(ranked) != NumPlayers-1 {
			t.Fatalf("seed %d: 結束時應有 3 位玩家排出名次，實際 %d", seed, len(ranked))
		}
		for i, p := range ranked {
			if p.Rank != i+1 {
				t.Fatalf("seed %d: 名次不連續，第 %d 位的 Rank = %d", seed, i+1, p.Rank)
			}
			if len(p.Hand) != 0 {
				t.Fatalf("seed %d: 第 %d 名手上還有牌", seed, p.Rank)
			}
		}
	}
}
