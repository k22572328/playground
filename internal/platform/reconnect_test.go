package platform

import (
	"strings"
	"testing"
	"time"
)

// TestResumeKeepsName 驗證重新整理後不必重新輸入暱稱。
func TestResumeKeepsName(t *testing.T) {
	ts := newTestServer(t)

	first := dial(t, ts)
	first.setName("阿明")
	token := first.token

	// 模擬關掉分頁：連線斷掉。
	first.conn.Close()
	time.Sleep(100 * time.Millisecond)

	// 新分頁帶著 token 回來。
	second := dial(t, ts)
	second.send(inbound{Action: actResume, Token: token})
	second.readUntil(msgLobby)

	// 名字還在 —— 開房時房名會用到它。
	second.send(inbound{Action: actCreateRoom, Name: ""})
	msg := second.readUntil(msgRoom)
	if !strings.Contains(msg.Room.Name, "阿明") {
		t.Errorf("重連後應保留暱稱，房名 = %q", msg.Room.Name)
	}
}

// TestResumeRejoinsGame 驗證遊戲進行中斷線重連能接回原本的座位與手牌。
func TestResumeRejoinsGame(t *testing.T) {
	ts := newTestServer(t)

	host, roomID := hostWithRoom(t, ts)
	guests := []*testClient{
		joinRoom(t, ts, "客人1", roomID),
		joinRoom(t, ts, "客人2", roomID),
		joinRoom(t, ts, "客人3", roomID),
	}
	host.send(inbound{Action: actStart})

	// 記下客人1 開局時的座位與手牌。
	victim := guests[0]
	view := drainUntilMatch(t, victim)
	wantSeat := view.YourSeat
	wantHand := len(view.YourHand)
	token := victim.token
	if wantHand != 13 {
		t.Fatalf("開局手牌應為 13 張，實際 %d", wantHand)
	}

	// 客人1 斷線。
	victim.conn.Close()
	time.Sleep(150 * time.Millisecond)

	// 其他人應該看到他離線，而且牌局暫停。
	drainUntilMatch(t, host)

	// 客人1 帶著 token 回來。
	back := dial(t, ts)
	back.send(inbound{Action: actResume, Token: token})

	got := drainUntilMatch(t, back)
	if got.YourSeat != wantSeat {
		t.Errorf("重連後座位 = %d，應該還是 %d", got.YourSeat, wantSeat)
	}
	if len(got.YourHand) != wantHand {
		t.Errorf("重連後手牌 %d 張，應該還是 %d 張", len(got.YourHand), wantHand)
	}
	if len(got.WaitingFor) != 0 {
		t.Errorf("人都回來了，不該還在等 %v", got.WaitingFor)
	}
}

// TestGamePausesWhileOffline 驗證有人斷線時牌局會暫停：
// 其他人看得到在等誰，而且不能繼續出牌。
func TestGamePausesWhileOffline(t *testing.T) {
	ts := newTestServer(t)

	host, roomID := hostWithRoom(t, ts)
	guests := []*testClient{
		joinRoom(t, ts, "客人1", roomID),
		joinRoom(t, ts, "客人2", roomID),
		joinRoom(t, ts, "客人3", roomID),
	}
	host.send(inbound{Action: actStart})
	for _, g := range guests {
		drainUntilMatch(t, g)
	}
	drainUntilMatch(t, host)

	// 客人1 斷線。
	guests[0].conn.Close()
	time.Sleep(150 * time.Millisecond)

	// 房長應該看到牌局暫停，並知道在等誰。
	var paused *matchView
	for range 6 {
		v := drainUntilMatch(t, host)
		if len(v.WaitingFor) > 0 {
			paused = v
			break
		}
	}
	if paused == nil {
		t.Fatal("有人斷線時應該顯示在等他重連")
	}
	if paused.WaitingFor[0] != "客人1" {
		t.Errorf("應該在等客人1，實際等 %v", paused.WaitingFor)
	}
	if paused.CanPlay || paused.CanPass {
		t.Error("牌局暫停時不該還能出牌或 PASS")
	}
	// 斷線者的座位要標記出來。
	offlineSeats := 0
	for _, s := range paused.Seats {
		if s.Offline {
			offlineSeats++
		}
	}
	if offlineSeats != 1 {
		t.Errorf("應有 1 個座位標記為斷線，實際 %d", offlineSeats)
	}
}

// TestResumeRejectsTakeover 驗證「保護先連上的人」：
// 原本的連線還活著時，別人拿著同一組 token 不能把身分搶走。
func TestResumeRejectsTakeover(t *testing.T) {
	ts := newTestServer(t)

	original := dial(t, ts)
	original.setName("先來的")
	token := original.token

	// 另一個視窗拿同一組 token 想接手 —— 原本那條還活著，應該被擋。
	intruder := dial(t, ts)
	intruder.send(inbound{Action: actResume, Token: token})
	msg := intruder.readUntil(msgError)
	if !strings.Contains(msg.Message, "別的視窗") {
		t.Errorf("錯誤訊息 = %q，應說明身分正在使用中", msg.Message)
	}

	// 先連上的那條完全不受影響，還能正常動作。
	original.send(inbound{Action: actCreateRoom, Name: "我的房"})
	if room := original.readUntil(msgRoom); !room.Room.YouAreHost {
		t.Error("原本的連線應該不受影響，仍是房長")
	}
}

// TestResumeWithBadTokenFails 驗證亂給或過期的 token 會被拒絕，
// 而不是讓人拿到別人的身分。
func TestResumeWithBadTokenFails(t *testing.T) {
	ts := newTestServer(t)

	for _, tc := range []struct{ name, token string }{
		{"空 token", ""},
		{"亂編的 token", "not-a-real-token"},
		{"長得像但不存在", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := dial(t, ts)
			c.send(inbound{Action: actResume, Token: tc.token})
			c.readUntil(msgError)
		})
	}
}

// TestResumeBeforeNamingFails 驗證還沒取名的身分沒什麼好接回的。
func TestResumeBeforeNamingFails(t *testing.T) {
	ts := newTestServer(t)

	first := dial(t, ts)
	token := first.token // 拿到 token 了，但還沒取名
	first.conn.Close()
	time.Sleep(100 * time.Millisecond)

	second := dial(t, ts)
	second.send(inbound{Action: actResume, Token: token})
	second.readUntil(msgError)
}

// TestDisconnectBeforeStartLeavesRoom 驗證還沒開局時斷線就直接離開房間 ——
// 這種情況留著座位只會擋住別人加入。
func TestDisconnectBeforeStartLeavesRoom(t *testing.T) {
	ts := newTestServer(t)

	host, roomID := hostWithRoom(t, ts)
	guest := joinRoom(t, ts, "客人", roomID)
	host.readUntil(msgRoom)

	guest.conn.Close()

	msg := host.readUntil(msgRoom)
	if len(msg.Room.Seats) != 1 {
		t.Errorf("開局前斷線應直接離開，房裡應剩 1 人，實際 %d", len(msg.Room.Seats))
	}
}
