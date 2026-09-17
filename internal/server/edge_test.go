package server

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestMalformedInputIsRejected 用各種畸形或惡意的輸入轟炸伺服器，
// 確認每一則都只換來一個錯誤訊息，而不是讓連線或伺服器掛掉。
func TestMalformedInputIsRejected(t *testing.T) {
	ts := newTestServer(t)
	c := dial(t, ts)
	c.setName("阿明")

	bad := []struct {
		name string
		msg  inbound
	}{
		{"空動作", inbound{}},
		{"不存在的動作", inbound{Action: "drop_table"}},
		{"加入不存在的房間", inbound{Action: actJoinRoom, RoomID: "ZZZ"}},
		{"加入空房號", inbound{Action: actJoinRoom, RoomID: ""}},
		{"不在房間時離開", inbound{Action: actLeaveRoom}},
		{"不在房間時開始", inbound{Action: actStart}},
		{"不在房間時出牌", inbound{Action: actPlay, Cards: []cardRef{{Rank: 0, Suit: 0}}}},
		{"不在房間時 PASS", inbound{Action: actPass}},
		{"不在房間時踢人", inbound{Action: actKick, TargetID: "p999"}},
		{"踢不存在的玩家", inbound{Action: actKick, TargetID: ""}},
		{"負數點數", inbound{Action: actPlay, Cards: []cardRef{{Rank: -1, Suit: 0}}}},
		{"超大點數", inbound{Action: actPlay, Cards: []cardRef{{Rank: 1 << 30, Suit: 0}}}},
		{"負數花色", inbound{Action: actPlay, Cards: []cardRef{{Rank: 0, Suit: -5}}}},
		{"超大花色", inbound{Action: actPlay, Cards: []cardRef{{Rank: 0, Suit: 999}}}},
	}

	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			c.send(tt.msg)
			msg := c.readUntil(msgError)
			if msg.Message == "" {
				t.Error("錯誤訊息不該是空的")
			}
		})
	}

	// 轟炸完之後連線仍然可用。
	c.send(inbound{Action: actCreateRoom, Name: "還活著"})
	c.readUntil(msgRoom)
}

// TestOversizedPayloadClosesConnection 驗證超大訊息會被擋下，
// 不會讓伺服器無上限地配置記憶體。
func TestOversizedPayloadClosesConnection(t *testing.T) {
	ts := newTestServer(t)
	c := dial(t, ts)

	// 遠超過 maxMessageBytes 的訊息。
	huge := inbound{Action: actSetName, Name: strings.Repeat("長", maxMessageBytes)}
	if err := c.conn.WriteJSON(huge); err != nil {
		// 連線可能已被關閉，這也算擋下來了。
		return
	}
	// 伺服器應該關掉這條連線，而不是照單全收。
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var msg outbound
	if err := c.conn.ReadJSON(&msg); err == nil {
		t.Error("超大訊息應該導致連線關閉或回報錯誤")
	}
}

// TestLongNameIsTruncated 驗證超長暱稱會被截斷，不會撐破版面。
func TestLongNameIsTruncated(t *testing.T) {
	ts := newTestServer(t)
	c := dial(t, ts)

	long := strings.Repeat("阿", 100)
	c.send(inbound{Action: actSetName, Name: long})
	c.readUntil(msgLobby)

	c.send(inbound{Action: actCreateRoom, Name: "房"})
	msg := c.readUntil(msgRoom)

	got := []rune(msg.Room.Seats[0].Name)
	if len(got) > 12 {
		t.Errorf("暱稱長度 %d，應該被截到 12 字以內", len(got))
	}
}

// TestNameWithOnlyWhitespaceRejected 驗證各種空白字元組成的暱稱都會被擋。
func TestNameWithOnlyWhitespaceRejected(t *testing.T) {
	ts := newTestServer(t)

	for _, name := range []string{"", " ", "\t", "\n", "   \t\n  "} {
		c := dial(t, ts)
		c.send(inbound{Action: actSetName, Name: name})
		c.readUntil(msgError)
	}
}

// TestDoubleJoinRejected 驗證同一個人不能重複加入同一間房。
func TestDoubleJoinRejected(t *testing.T) {
	ts := newTestServer(t)
	_, roomID := hostWithRoom(t, ts)

	guest := dial(t, ts)
	guest.setName("客人")
	guest.send(inbound{Action: actJoinRoom, RoomID: roomID})
	guest.readUntil(msgRoom)

	// 再加入一次就該被擋。
	guest.send(inbound{Action: actJoinRoom, RoomID: roomID})
	guest.readUntil(msgError)
}

// TestFifthPlayerRejected 驗證第五個人加不進滿房。
func TestFifthPlayerRejected(t *testing.T) {
	ts := newTestServer(t)
	_, roomID := hostWithRoom(t, ts)
	joinRoom(t, ts, "客人1", roomID)
	joinRoom(t, ts, "客人2", roomID)
	joinRoom(t, ts, "客人3", roomID)

	fifth := dial(t, ts)
	fifth.setName("第五人")
	fifth.send(inbound{Action: actJoinRoom, RoomID: roomID})
	if msg := fifth.readUntil(msgError); !strings.Contains(msg.Message, "已滿") {
		t.Errorf("錯誤訊息 = %q，應提到房間已滿", msg.Message)
	}
}

// TestJoinStartedRoomRejected 驗證遊戲開始後就不能再加入。
func TestJoinStartedRoomRejected(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guests := []*testClient{
		joinRoom(t, ts, "客人1", roomID),
		joinRoom(t, ts, "客人2", roomID),
		joinRoom(t, ts, "客人3", roomID),
	}
	host.send(inbound{Action: actStart})
	drainUntilMatch(t, host)
	for _, g := range guests {
		drainUntilMatch(t, g)
	}

	// 有人離開後空出位子，但遊戲已開始，仍不能加入。
	late := dial(t, ts)
	late.setName("遲到")
	late.send(inbound{Action: actJoinRoom, RoomID: roomID})
	late.readUntil(msgError)
}

// TestConcurrentActionsFromManyClients 讓大量連線同時亂送動作，
// 確認伺服器不會 panic、死鎖或錯亂。這是最接近真實混亂情況的測試。
func TestConcurrentActionsFromManyClients(t *testing.T) {
	ts := newTestServer(t)

	const clients = 30
	var wg sync.WaitGroup
	for i := range clients {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			// 持續讀，避免送信佇列塞滿而被伺服器斷線。
			go func() {
				for {
					var msg outbound
					if err := conn.ReadJSON(&msg); err != nil {
						return
					}
				}
			}()

			name := "玩家" + itoa(int64(i))
			actions := []inbound{
				{Action: actSetName, Name: name},
				{Action: actCreateRoom, Name: name + "的房"},
				{Action: actJoinRoom, RoomID: "AAA"},
				{Action: actStart},
				{Action: actPass},
				{Action: actKick, TargetID: "p1"},
				{Action: actLeaveRoom},
				{Action: "亂送的動作"},
			}
			for _, a := range actions {
				if err := conn.WriteJSON(a); err != nil {
					return
				}
			}
		}(i)
	}
	wg.Wait()

	// 亂七八糟之後，伺服器仍然能正常服務新連線。
	c := dial(t, ts)
	c.setName("收尾")
	c.send(inbound{Action: actCreateRoom, Name: "最後一間"})
	c.readUntil(msgRoom)
}

// TestDisconnectLeavesRoom 驗證玩家斷線後會自動離開房間，
// 不會留下佔位的幽靈玩家。
func TestDisconnectLeavesRoom(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guest := joinRoom(t, ts, "客人", roomID)
	host.readUntil(msgRoom)

	// 客人直接斷線。
	guest.conn.Close()

	// 房長應該收到房間更新，人數回到 1。
	msg := host.readUntil(msgRoom)
	if len(msg.Room.Seats) != 1 {
		t.Errorf("斷線後房裡應剩 1 人，實際 %d", len(msg.Room.Seats))
	}
}
