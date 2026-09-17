package platform

import (
	"math/rand"
	"testing"

	"playground/internal/games/bigtwo/match"
)

func newTestLobby() *Lobby { return newLobby(rand.New(rand.NewSource(1))) }

func seat(id, name string) Seat { return Seat{PlayerID: id, Name: name} }

// fill 把房間補滿到四人，回傳房間。
func fill(t *testing.T, l *Lobby, r *Room) *Room {
	t.Helper()
	for _, s := range []Seat{seat("p2", "B"), seat("p3", "C"), seat("p4", "D")} {
		if _, err := l.Join(r.ID, s); err != nil {
			t.Fatalf("Join(%s) 失敗: %v", s.PlayerID, err)
		}
	}
	return r
}

func TestCreateMakesCreatorHost(t *testing.T) {
	l := newTestLobby()
	r := l.Create("測試房", seat("p1", "A"))

	if !r.IsHost("p1") {
		t.Error("建立者應該是房長")
	}
	if len(r.Seats) != 1 {
		t.Errorf("新房間應該只有 1 人，實際 %d", len(r.Seats))
	}
	if r.Started {
		t.Error("新房間不該是已開始狀態")
	}
}

func TestListShowsCreatedRooms(t *testing.T) {
	l := newTestLobby()
	l.Create("房一", seat("p1", "A"))
	l.Create("房二", seat("p2", "B"))

	if got := len(l.List()); got != 2 {
		t.Errorf("大廳應該有 2 個房間，實際 %d", got)
	}
}

func TestJoinUntilFull(t *testing.T) {
	l := newTestLobby()
	r := fill(t, l, l.Create("測試房", seat("p1", "A")))

	if !r.Full() {
		t.Fatal("四人應該算滿房")
	}
	// 第五人進不來。
	if _, err := l.Join(r.ID, seat("p5", "E")); err != ErrRoomFull {
		t.Errorf("err = %v, 想要 %v", err, ErrRoomFull)
	}
}

func TestCannotJoinTwice(t *testing.T) {
	l := newTestLobby()
	r := l.Create("測試房", seat("p1", "A"))

	if _, err := l.Join(r.ID, seat("p1", "A")); err != ErrAlreadyJoined {
		t.Errorf("err = %v, 想要 %v", err, ErrAlreadyJoined)
	}
}

func TestJoinUnknownRoom(t *testing.T) {
	l := newTestLobby()
	if _, err := l.Join("ZZZ", seat("p1", "A")); err != ErrRoomNotFound {
		t.Errorf("err = %v, 想要 %v", err, ErrRoomNotFound)
	}
}

func TestHostSuccession(t *testing.T) {
	l := newTestLobby()
	r := l.Create("測試房", seat("p1", "A"))
	if _, err := l.Join(r.ID, seat("p2", "B")); err != nil {
		t.Fatalf("Join 失敗: %v", err)
	}

	if err := l.Leave(r.ID, "p1"); err != nil {
		t.Fatalf("Leave 失敗: %v", err)
	}
	if !r.IsHost("p2") {
		t.Error("房長離開後應由下一位遞補")
	}
}

func TestEmptyRoomIsDeleted(t *testing.T) {
	l := newTestLobby()
	r := l.Create("測試房", seat("p1", "A"))

	if err := l.Leave(r.ID, "p1"); err != nil {
		t.Fatalf("Leave 失敗: %v", err)
	}
	if _, err := l.Get(r.ID); err != ErrRoomNotFound {
		t.Errorf("空房間應該被刪除，err = %v", err)
	}
}

func TestKick(t *testing.T) {
	l := newTestLobby()
	r := l.Create("測試房", seat("p1", "A"))
	if _, err := l.Join(r.ID, seat("p2", "B")); err != nil {
		t.Fatalf("Join 失敗: %v", err)
	}

	// 非房長不能踢人。
	if err := l.Kick(r.ID, "p2", "p1"); err != ErrNotHost {
		t.Errorf("非房長踢人 err = %v, 想要 %v", err, ErrNotHost)
	}
	// 房長不能踢自己。
	if err := l.Kick(r.ID, "p1", "p1"); err != ErrCannotKickSelf {
		t.Errorf("房長踢自己 err = %v, 想要 %v", err, ErrCannotKickSelf)
	}
	// 房長踢掉別人。
	if err := l.Kick(r.ID, "p1", "p2"); err != nil {
		t.Fatalf("房長踢人失敗: %v", err)
	}
	if len(r.Seats) != 1 {
		t.Errorf("踢人後應剩 1 人，實際 %d", len(r.Seats))
	}
}

func TestStartRequiresFullRoom(t *testing.T) {
	l := newTestLobby()
	r := l.Create("測試房", seat("p1", "A"))

	if _, err := l.Start(r.ID, "p1"); err != ErrNotEnough {
		t.Errorf("人數不足時 err = %v, 想要 %v", err, ErrNotEnough)
	}
}

func TestStartRequiresHost(t *testing.T) {
	l := newTestLobby()
	r := fill(t, l, l.Create("測試房", seat("p1", "A")))

	if _, err := l.Start(r.ID, "p2"); err != ErrNotHost {
		t.Errorf("非房長開始遊戲 err = %v, 想要 %v", err, ErrNotHost)
	}
}

// TestStartShufflesSeats 驗證開局時座位會重新洗過，不照進房順序。
// 連開多局，只要看到任何一局的座位排列與進房順序不同就算通過 ——
// 剛好洗回原順序的機率是 1/24，單看一局無法判斷。
func TestStartShufflesSeats(t *testing.T) {
	joinOrder := []string{"host", "p2", "p3", "p4"}

	shuffled := false
	for seed := range 50 {
		l := newLobby(rand.New(rand.NewSource(int64(seed))))
		r := fill(t, l, l.Create("洗座位", seat("host", "房長")))
		if _, err := l.Start(r.ID, "host"); err != nil {
			t.Fatalf("Start 失敗: %v", err)
		}

		// 不論怎麼洗，四個人都必須各出現一次。
		seen := make(map[string]int)
		for _, s := range r.Seats {
			seen[s.PlayerID]++
		}
		if len(seen) != match.NumPlayers {
			t.Fatalf("seed %d: 洗完只剩 %d 位玩家", seed, len(seen))
		}
		for id, n := range seen {
			if n != 1 {
				t.Fatalf("seed %d: 玩家 %s 出現 %d 次", seed, id, n)
			}
		}

		for i, s := range r.Seats {
			if s.PlayerID != joinOrder[i] {
				shuffled = true
			}
		}
	}

	if !shuffled {
		t.Error("連開 50 局座位順序都與進房順序相同，看起來沒有洗牌")
	}
}

// TestHostSurvivesSeatShuffle 驗證洗座位不會把房長身分洗掉 ——
// 房長是特定的人，不是「坐在座位 0 的人」。
func TestHostSurvivesSeatShuffle(t *testing.T) {
	for seed := range 50 {
		l := newLobby(rand.New(rand.NewSource(int64(seed))))
		r := fill(t, l, l.Create("洗座位", seat("host", "房長")))
		if _, err := l.Start(r.ID, "host"); err != nil {
			t.Fatalf("Start 失敗: %v", err)
		}
		if !r.IsHost("host") {
			t.Fatalf("seed %d: 洗完座位後房長身分跑掉了", seed)
		}
		// 其他人不該因為被洗到座位 0 就變成房長。
		for _, id := range []string{"p2", "p3", "p4"} {
			if r.IsHost(id) {
				t.Fatalf("seed %d: %s 不該是房長", seed, id)
			}
		}
	}
}

func TestStartDealsMatch(t *testing.T) {
	l := newTestLobby()
	r := fill(t, l, l.Create("測試房", seat("p1", "A")))

	m, err := l.Start(r.ID, "p1")
	if err != nil {
		t.Fatalf("Start 失敗: %v", err)
	}
	if !r.Started {
		t.Error("開始後房間應標記為已開始")
	}
	for _, p := range m.Players {
		if len(p.Hand) != 13 {
			t.Errorf("每人應發 13 張，座位 %d 實際 %d", p.Seat, len(p.Hand))
		}
	}
	// 開始後不能再加入或踢人。
	if _, err := l.Join(r.ID, seat("p5", "E")); err != ErrRoomStarted {
		t.Errorf("已開始的房間 Join err = %v, 想要 %v", err, ErrRoomStarted)
	}
	if err := l.Kick(r.ID, "p1", "p2"); err != ErrRoomStarted {
		t.Errorf("已開始的房間 Kick err = %v, 想要 %v", err, ErrRoomStarted)
	}
	if _, err := l.Start(r.ID, "p1"); err != ErrRoomStarted {
		t.Errorf("重複開始 err = %v, 想要 %v", err, ErrRoomStarted)
	}
}

// TestRoomIDsAreUnique 確認房號不會撞號。
func TestRoomIDsAreUnique(t *testing.T) {
	l := newTestLobby()
	seen := make(map[string]bool)
	for i := range 200 {
		r := l.Create("房", seat("p", "A"))
		if seen[r.ID] {
			t.Fatalf("第 %d 個房間的房號 %s 重複了", i, r.ID)
		}
		seen[r.ID] = true
	}
}
