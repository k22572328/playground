package lobby

import (
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"testing"

	"bigTwo/internal/game"
	"bigTwo/internal/match"
)

// TestConcurrentJoinNeverOverfills 讓大量玩家同時搶同一間房，
// 驗證「最多四人」這個上限在並行下不會被突破。
func TestConcurrentJoinNeverOverfills(t *testing.T) {
	const contenders = 200

	for round := range 50 {
		l := newLobby()
		r := l.Create("搶位子", seat("host", "房長"))

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
		if want := match.NumPlayers - 1; got != want {
			t.Fatalf("第 %d 回合：成功加入 %d 人，應該剛好 %d 人", round, got, want)
		}
		if len(r.Seats) != match.NumPlayers {
			t.Fatalf("第 %d 回合：房內 %d 人，應該剛好 %d 人", round, len(r.Seats), match.NumPlayers)
		}
	}
}

// TestConcurrentStartOnlyOnce 讓房長重複並行按下開始，
// 驗證只會真的開出一局，不會重複發牌。
func TestConcurrentStartOnlyOnce(t *testing.T) {
	for round := range 50 {
		l := newLobby()
		r := fill(t, l, l.Create("搶開始", seat("host", "房長")))

		var mu sync.Mutex
		var started []*match.Match

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
	l := newLobby()
	r := l.Create("進進出出", seat("host", "房長"))
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
	if len(room.Seats) > match.NumPlayers {
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

// TestConcurrentPlayKeepsMatchConsistent 讓四位玩家同時搶著出牌，
// 驗證牌局狀態不會被並行寫壞：手牌總數必須始終守恆。
func TestConcurrentPlayKeepsMatchConsistent(t *testing.T) {
	l := New(rand.New(rand.NewSource(7)))
	r := fill(t, l, l.Create("搶出牌", seat("host", "房長")))
	if _, err := l.Start(r.ID, "host"); err != nil {
		t.Fatalf("Start 失敗: %v", err)
	}

	// 先記下每個人的手牌，讓各 goroutine 拿真牌去搶出。
	hands := make(map[string][]game.Card)
	ids := []string{"host", "p2", "p3", "p4"}
	for i, id := range ids {
		hands[id] = append([]game.Card(nil), r.Match.Players[i].Hand...)
	}

	var wg sync.WaitGroup
	for range 100 {
		for _, id := range ids {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				// 四家同時搶著出自己手上的第一張牌，外加 PASS。
				// 絕大多數會被規則擋下，重點是並行不會讓狀態錯亂或 panic。
				if h := hands[id]; len(h) > 0 {
					_ = l.PlayInRoom(r.ID, id, h[:1])
				}
				_ = l.PlayInRoom(r.ID, id, nil)
			}(id)
		}
	}
	wg.Wait()

	// 不論誰成功出了什麼，沒有人的手牌會超過原本的張數，
	// 也不會有牌憑空出現。所有 goroutine 都結束了，這裡可以直接讀。
	total := 0
	for i, p := range r.Match.Players {
		if len(p.Hand) > match.CardsPerHand {
			t.Errorf("座位 %d 手牌 %d 張，超過發牌數", i, len(p.Hand))
		}
		// 每個人手上的牌必須都還是原本發到的那些，不能冒出別人的牌。
		for _, c := range p.Hand {
			if !slices.Contains(hands[ids[i]], c) {
				t.Errorf("座位 %d 手上出現不屬於他的牌 %v", i, c)
			}
		}
		total += len(p.Hand)
	}
	if total > match.NumPlayers*match.CardsPerHand {
		t.Errorf("手牌總數 %d 超過 52 張", total)
	}
}
