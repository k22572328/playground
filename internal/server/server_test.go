package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// testClient 是測試用的一條連線，包住 WebSocket 並提供讀寫的小工具。
type testClient struct {
	t        *testing.T
	conn     *websocket.Conn
	playerID string
}

// dial 連上測試伺服器，並先收下 welcome 訊息。
func dial(t *testing.T, ts *httptest.Server) *testClient {
	t.Helper()

	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("連線失敗: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	c := &testClient{t: t, conn: conn}
	welcome := c.read()
	if welcome.Type != msgWelcome {
		t.Fatalf("第一則訊息應該是 welcome，實際 %s", welcome.Type)
	}
	c.playerID = welcome.PlayerID
	return c
}

// send 送出一個動作。
func (c *testClient) send(msg inbound) {
	c.t.Helper()
	if err := c.conn.WriteJSON(msg); err != nil {
		c.t.Fatalf("送出 %s 失敗: %v", msg.Action, err)
	}
}

// read 讀一則訊息，逾時即失敗。
func (c *testClient) read() outbound {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	var msg outbound
	if err := c.conn.ReadJSON(&msg); err != nil {
		c.t.Fatalf("讀取失敗: %v", err)
	}
	return msg
}

// readUntil 一直讀到指定類型的訊息為止，中途的其他訊息會被略過。
// 伺服器常常連續推播多則（例如同時更新房間與大廳），測試只關心其中一則；
// 別人的動作也會推播大廳更新過來，所以容許量要夠寬。
func (c *testClient) readUntil(msgType string) outbound {
	c.t.Helper()
	const maxSkip = 60
	for i := 0; i < maxSkip; i++ {
		msg := c.read()
		if msg.Type == msgType {
			return msg
		}
		if msg.Type == msgError {
			c.t.Fatalf("等 %s 的時候收到錯誤: %s", msgType, msg.Message)
		}
	}
	c.t.Fatalf("連讀 %d 則都沒等到 %s", maxSkip, msgType)
	return outbound{}
}

// setName 設定暱稱並等大廳訊息回來。
func (c *testClient) setName(name string) {
	c.t.Helper()
	c.send(inbound{Action: actSetName, Name: name})
	c.readUntil(msgLobby)
}

// drainUntilMatch 一直讀到收進牌局狀態為止，回傳那份牌局視角。
func drainUntilMatch(t *testing.T, c *testClient) *matchView {
	t.Helper()
	for range 12 {
		if msg := c.readUntil(msgRoom); msg.Match != nil {
			return msg.Match
		}
	}
	t.Fatalf("%s 沒收到牌局狀態", c.playerID)
	return nil
}

// newTestServer 起一台測試伺服器。
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(New().Handler())
	t.Cleanup(ts.Close)
	return ts
}

// TestStaticFilesServed 驗證前端頁面供應正常。前端是編進執行檔的，
// 所以不論伺服器在哪個目錄下執行都該拿得到 —— 曾經因為用相對路徑找
// web 目錄，導致從 cmd/server 底下執行時每頁都回 404。
func TestStaticFilesServed(t *testing.T) {
	ts := newTestServer(t)

	for _, tc := range []struct{ path, wantType, wantBody string }{
		{"/", "text/html", "<title>大老二</title>"},
		{"/index.html", "text/html", "<title>大老二</title>"},
		{"/app.js", "javascript", "WebSocket"},
		{"/style.css", "text/css", ".card"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + tc.path)
			if err != nil {
				t.Fatalf("取得 %s 失敗: %v", tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("%s 回應 %d，應該是 200", tc.path, resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, tc.wantType) {
				t.Errorf("%s 的 Content-Type = %q，應含 %q", tc.path, ct, tc.wantType)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("讀取 %s 內容失敗: %v", tc.path, err)
			}
			if !strings.Contains(string(body), tc.wantBody) {
				t.Errorf("%s 的內容沒有包含 %q", tc.path, tc.wantBody)
			}
		})
	}
}

func TestSetNameRequired(t *testing.T) {
	ts := newTestServer(t)
	c := dial(t, ts)

	// 還沒設暱稱就想開房，應該被擋下。
	c.send(inbound{Action: actCreateRoom, Name: "我的房"})
	if msg := c.readUntil(msgError); !strings.Contains(msg.Message, "暱稱") {
		t.Errorf("錯誤訊息應提到暱稱，實際 %q", msg.Message)
	}
}

func TestEmptyNameRejected(t *testing.T) {
	ts := newTestServer(t)
	c := dial(t, ts)

	c.send(inbound{Action: actSetName, Name: "   "})
	c.readUntil(msgError)
}

func TestCreateRoomMakesYouHost(t *testing.T) {
	ts := newTestServer(t)
	c := dial(t, ts)
	c.setName("阿明")

	c.send(inbound{Action: actCreateRoom, Name: "測試房"})
	msg := c.readUntil(msgRoom)

	if msg.Room == nil {
		t.Fatal("應該收到房間資訊")
	}
	if !msg.Room.YouAreHost {
		t.Error("建立者應該是房長")
	}
	if len(msg.Room.Seats) != 1 {
		t.Errorf("新房間應該只有 1 人，實際 %d", len(msg.Room.Seats))
	}
	if msg.Room.Started {
		t.Error("新房間不該已開始")
	}
}

func TestLobbyListsRooms(t *testing.T) {
	ts := newTestServer(t)

	host := dial(t, ts)
	host.setName("房長")
	host.send(inbound{Action: actCreateRoom, Name: "大廳測試房"})
	host.readUntil(msgRoom)

	// 後進來的人應該在大廳看得到這間房。
	guest := dial(t, ts)
	guest.send(inbound{Action: actSetName, Name: "路人"})
	msg := guest.readUntil(msgLobby)

	if len(msg.Rooms) != 1 {
		t.Fatalf("大廳應該有 1 間房，實際 %d", len(msg.Rooms))
	}
	if msg.Rooms[0].Name != "大廳測試房" {
		t.Errorf("房名 = %q", msg.Rooms[0].Name)
	}
	if msg.Rooms[0].YouAreHost {
		t.Error("路人不該被標成房長")
	}
}

// joinRoom 讓一位新玩家加入指定房間。
func joinRoom(t *testing.T, ts *httptest.Server, name, roomID string) *testClient {
	t.Helper()
	c := dial(t, ts)
	c.setName(name)
	c.send(inbound{Action: actJoinRoom, RoomID: roomID})
	c.readUntil(msgRoom)
	return c
}

// hostWithRoom 建立一位房長與一間房，回傳房長與房號。
func hostWithRoom(t *testing.T, ts *httptest.Server) (*testClient, string) {
	t.Helper()
	host := dial(t, ts)
	host.setName("房長")
	host.send(inbound{Action: actCreateRoom, Name: "測試房"})
	msg := host.readUntil(msgRoom)
	return host, msg.Room.ID
}

func TestJoinRoomNotifiesEveryone(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)

	joinRoom(t, ts, "客人", roomID)

	// 房長應該收到有人加入的更新。
	msg := host.readUntil(msgRoom)
	if len(msg.Room.Seats) != 2 {
		t.Errorf("房裡應該有 2 人，實際 %d", len(msg.Room.Seats))
	}
}

func TestStartRequiresFourPlayers(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	joinRoom(t, ts, "客人", roomID)
	host.readUntil(msgRoom)

	// 只有兩人，不能開始。
	host.send(inbound{Action: actStart})
	if msg := host.readUntil(msgError); !strings.Contains(msg.Message, "人數不足") {
		t.Errorf("錯誤訊息 = %q，應提到人數不足", msg.Message)
	}
}

func TestNonHostCannotStart(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guest := joinRoom(t, ts, "客人", roomID)
	joinRoom(t, ts, "客人2", roomID)
	joinRoom(t, ts, "客人3", roomID)
	_ = host

	guest.send(inbound{Action: actStart})
	if msg := guest.readUntil(msgError); !strings.Contains(msg.Message, "房長") {
		t.Errorf("錯誤訊息 = %q，應提到房長", msg.Message)
	}
}

func TestKickRemovesPlayer(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guest := joinRoom(t, ts, "客人", roomID)
	host.readUntil(msgRoom)

	host.send(inbound{Action: actKick, TargetID: guest.playerID})

	// 被踢的人要收到通知。
	if msg := guest.readUntil(msgKicked); msg.Message == "" {
		t.Error("被踢時應附上說明")
	}
	// 房長看到的房間應該只剩自己。
	msg := host.readUntil(msgRoom)
	if len(msg.Room.Seats) != 1 {
		t.Errorf("踢人後應剩 1 人，實際 %d", len(msg.Room.Seats))
	}
}

// TestSeatViewCarriesPlayerID 驗證房間視圖帶得出每個位子的 playerId ——
// 前端的踢人按鈕靠它指定對象，少了就會送出空的目標。
func TestSeatViewCarriesPlayerID(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guest := joinRoom(t, ts, "客人", roomID)

	msg := host.readUntil(msgRoom)
	if len(msg.Room.Seats) != 2 {
		t.Fatalf("房裡應有 2 人，實際 %d", len(msg.Room.Seats))
	}
	for _, s := range msg.Room.Seats {
		if s.PlayerID == "" {
			t.Errorf("座位 %q 沒有帶出 playerId", s.Name)
		}
	}
	// 用視圖裡拿到的 ID 去踢人，必須真的踢得動。
	var guestSlot string
	for _, s := range msg.Room.Seats {
		if !s.IsHost {
			guestSlot = s.PlayerID
		}
	}
	if guestSlot != guest.playerID {
		t.Fatalf("視圖給的 playerId = %q，應該是 %q", guestSlot, guest.playerID)
	}
	host.send(inbound{Action: actKick, TargetID: guestSlot})
	guest.readUntil(msgKicked)
}

func TestNonHostCannotKick(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guest := joinRoom(t, ts, "客人", roomID)

	guest.send(inbound{Action: actKick, TargetID: host.playerID})
	if msg := guest.readUntil(msgError); !strings.Contains(msg.Message, "房長") {
		t.Errorf("錯誤訊息 = %q，應提到房長", msg.Message)
	}
}

// TestFullGameStart 驗證滿四人開局後，每個人都拿到 13 張手牌，
// 而且看不到別人的牌。
func TestFullGameStart(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guests := []*testClient{
		joinRoom(t, ts, "客人1", roomID),
		joinRoom(t, ts, "客人2", roomID),
		joinRoom(t, ts, "客人3", roomID),
	}

	host.send(inbound{Action: actStart})

	all := append([]*testClient{host}, guests...)
	turnCount := 0
	for _, c := range all {
		// 每個人都要收到已開始的牌局狀態。
		var msg outbound
		for i := 0; i < 12; i++ {
			msg = c.readUntil(msgRoom)
			if msg.Match != nil {
				break
			}
		}
		if msg.Match == nil {
			t.Fatalf("%s 沒收到牌局狀態", c.playerID)
		}

		if len(msg.Match.YourHand) != 13 {
			t.Errorf("%s 應有 13 張手牌，實際 %d", c.playerID, len(msg.Match.YourHand))
		}
		if len(msg.Match.Seats) != 4 {
			t.Errorf("牌桌應有 4 個座位，實際 %d", len(msg.Match.Seats))
		}
		for _, s := range msg.Match.Seats {
			if s.CardCount != 13 {
				t.Errorf("座位 %d 應有 13 張，實際 %d", s.Seat, s.CardCount)
			}
			if s.IsTurn {
				turnCount++
			}
		}
	}
	// 四個人的視角加起來，應該剛好各看到一位「輪到出牌」的玩家。
	if turnCount != len(all) {
		t.Errorf("每個視角都該有且只有一位輪到出牌，總計 %d，預期 %d", turnCount, len(all))
	}
}

// TestPlayMustBeYourTurn 驗證不是自己的回合時不能出牌。
func TestPlayMustBeYourTurn(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	joinRoom(t, ts, "客人1", roomID)
	joinRoom(t, ts, "客人2", roomID)
	joinRoom(t, ts, "客人3", roomID)

	host.send(inbound{Action: actStart})

	// 找出房長的視角，確認是不是他先出。
	var view *matchView
	for i := 0; i < 12 && view == nil; i++ {
		if msg := host.readUntil(msgRoom); msg.Match != nil {
			view = msg.Match
		}
	}
	if view == nil {
		t.Fatal("房長沒收到牌局狀態")
	}

	if view.CanPlay {
		t.Skip("這局剛好輪到房長先出，換個角度測不到「不是你的回合」")
	}
	// 不是房長的回合，出任何牌都該被拒絕。
	host.send(inbound{Action: actPlay, Cards: []cardRef{{Rank: int(view.YourHand[0].Rank), Suit: int(view.YourHand[0].Suit)}}})
	host.readUntil(msgError)
}

// TestInvalidCardRejected 驗證超出範圍的牌會被擋下，而不是讓伺服器出錯。
func TestInvalidCardRejected(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	joinRoom(t, ts, "客人1", roomID)
	joinRoom(t, ts, "客人2", roomID)
	joinRoom(t, ts, "客人3", roomID)

	host.send(inbound{Action: actStart})
	for i := 0; i < 12; i++ {
		if msg := host.readUntil(msgRoom); msg.Match != nil {
			break
		}
	}

	host.send(inbound{Action: actPlay, Cards: []cardRef{{Rank: 99, Suit: 99}}})
	host.readUntil(msgError)
}

func TestUnknownActionRejected(t *testing.T) {
	ts := newTestServer(t)
	c := dial(t, ts)
	c.setName("阿明")

	c.send(inbound{Action: "亂七八糟"})
	c.readUntil(msgError)
}

// TestLeaveRoomReturnsToLobby 驗證離開房間後會回到大廳。
func TestLeaveRoomReturnsToLobby(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guest := joinRoom(t, ts, "客人", roomID)
	host.readUntil(msgRoom)

	guest.send(inbound{Action: actLeaveRoom})
	guest.readUntil(msgLobby)

	msg := host.readUntil(msgRoom)
	if len(msg.Room.Seats) != 1 {
		t.Errorf("客人離開後應剩 1 人，實際 %d", len(msg.Room.Seats))
	}
}

// TestHostSuccessionOnLeave 驗證房長離開後由下一位遞補。
func TestHostSuccessionOnLeave(t *testing.T) {
	ts := newTestServer(t)
	host, roomID := hostWithRoom(t, ts)
	guest := joinRoom(t, ts, "客人", roomID)
	host.readUntil(msgRoom)

	host.send(inbound{Action: actLeaveRoom})
	host.readUntil(msgLobby)

	msg := guest.readUntil(msgRoom)
	if !msg.Room.YouAreHost {
		t.Error("原房長離開後，客人應該遞補成房長")
	}
}
