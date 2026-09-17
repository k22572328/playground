package platform

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"testing"
)

// testSeats 是假遊戲的人數，平台的壓力測試用它當上限。
const testSeats = 4

// TestConcurrentJoinNeverOverfills 讓大量玩家同時搶同一間房，
// 驗證「最多四人」這個上限在並行下不會被突破。
func TestConcurrentJoinNeverOverfills(t *testing.T) {
	const contenders = 200

	for round := range 50 {
		l := newTestLobby()
		r := mustCreate(t, l, "搶位子", seat("host", "房長"))

		var wg sync.WaitGroup
		var joined sync.Map
		for i := range contenders {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				id := fmt.Sprintf("p%d", i)
				if _, err := l.Join(r.ID, seat(id, id)); err == nil {
					joined.Store(id, true)
				}
			}(i)
		}
		wg.Wait()

		got := 0
		joined.Range(func(any, any) bool { got++; return true })
		// 房長已佔一位，所以最多只能再進三人。
		if want := testSeats - 1; got != want {
			t.Fatalf("第 %d 回合：成功加入 %d 人，應該剛好 %d 人", round, got, want)
		}
		if len(r.Seats) != testSeats {
			t.Fatalf("第 %d 回合：房內 %d 人，應該剛好 %d 人", round, len(r.Seats), testSeats)
		}
	}
}

// TestConcurrentStartOnlyOnce 讓房長重複並行按下開始，
// 驗證只會真的開出一局，不會重複發牌。
func TestConcurrentStartOnlyOnce(t *testing.T) {
	for round := range 50 {
		l := newTestLobby()
		r := fill(t, l, mustCreate(t, l, "搶開始", seat("host", "房長")))

		var mu sync.Mutex
		var started []Instance

		var wg sync.WaitGroup
		for range 50 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if m, err := l.Start(r.ID, "host"); err == nil {
					mu.Lock()
					started = append(started, m)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		if len(started) != 1 {
			t.Fatalf("第 %d 回合：成功開始 %d 次，應該只有 1 次", round, len(started))
		}
	}
}

// TestConcurrentLeaveAndJoin 混合並行的加入與離開，驗證房間狀態不會壞掉：
// 座位數永遠在合法範圍內，且不會出現重複的玩家。
func TestConcurrentLeaveAndJoin(t *testing.T) {
	l := newTestLobby()
	r := mustCreate(t, l, "進進出出", seat("host", "房長"))
	roomID := r.ID

	var wg sync.WaitGroup
	for i := range 300 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("p%d", i%8) // 刻意讓少數幾個 ID 反覆進出
			if _, err := l.Join(roomID, seat(id, id)); err == nil {
				_ = l.Leave(roomID, id)
			}
		}(i)
	}
	wg.Wait()

	// 房長從沒離開，所以房間必定還在。
	room, err := l.Get(roomID)
	if err != nil {
		t.Fatalf("房間不該消失: %v", err)
	}
	if len(room.Seats) > testSeats {
		t.Errorf("座位數 %d 超過上限", len(room.Seats))
	}
	seen := make(map[string]bool)
	for _, s := range room.Seats {
		if seen[s.PlayerID] {
			t.Errorf("玩家 %s 在房裡出現兩次", s.PlayerID)
		}
		seen[s.PlayerID] = true
	}
}

// TestConcurrentActsStayConsistent 讓四位玩家同時搶著做動作，
// 驗證平台不會把牌局狀態寫壞：不論誰搶到，動作總數不會超過遊戲允許的量，
// 而且過程中不會 panic 或死鎖。
func TestConcurrentActsStayConsistent(t *testing.T) {
	registerTestGame()
	l := newLobby(rand.New(rand.NewSource(7)))
	r := fill(t, l, mustCreate(t, l, "搶動作", seat("host", "房長")))
	if _, err := l.Start(r.ID, "host"); err != nil {
		t.Fatalf("Start 失敗: %v", err)
	}

	ids := []string{"host", "p2", "p3", "p4"}
	var wg sync.WaitGroup
	for range 100 {
		for _, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				// 大多數會因為「還沒輪到你」被擋下，
				// 重點是同時呼叫不會讓狀態錯亂。
				_ = l.ActInRoom(r.ID, id, json.RawMessage(`{"note":"x"}`))
			}(id)
		}
	}
	wg.Wait()

	// 所有 goroutine 都結束了，這裡可以直接讀。
	// 假遊戲規定每人各做一次就結束，所以最後必定是結束狀態。
	if !r.Game.Over() {
		t.Error("四個人各搶了上百次，這局早該結束了")
	}
}
