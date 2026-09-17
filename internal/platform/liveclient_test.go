package platform

import (
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// liveClient 是一條自己在背景持續收訊息的連線，就像真實的瀏覽器分頁：
// 不必先知道會收到幾則推播，隨時可以查詢「我現在看到的狀態」。
type liveClient struct {
	t    *testing.T
	conn *websocket.Conn

	mu       sync.Mutex
	playerID string
	match    *testView
	room     *roomView
	errs     []string
	closed   bool
}

// dialLive 連上伺服器並開始在背景收訊息。
func dialLive(t *testing.T, url string) *liveClient {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("連線失敗: %v", err)
	}

	c := &liveClient{t: t, conn: conn}
	t.Cleanup(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		conn.Close()
	})

	go c.readLoop()
	return c
}

// readLoop 持續把伺服器推播收進來，更新這個客戶端看到的狀態。
func (c *liveClient) readLoop() {
	for {
		var msg outbound
		if err := c.conn.ReadJSON(&msg); err != nil {
			return // 連線關閉
		}

		c.mu.Lock()
		switch msg.Type {
		case msgWelcome:
			c.playerID = msg.PlayerID
		case msgRoom:
			c.room = msg.Room
			if msg.Game != nil {
				c.match = gameView(c.t, msg.Game)
			}
		case msgError:
			c.errs = append(c.errs, msg.Message)
		}
		c.mu.Unlock()
	}
}

func (c *liveClient) send(msg inbound) {
	c.t.Helper()
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return
	}
	if err := c.conn.WriteJSON(msg); err != nil {
		c.t.Fatalf("送出 %s 失敗: %v", msg.Action, err)
	}
}

// id 等到 welcome 到達後回傳這位玩家的識別碼。
func (c *liveClient) id() string {
	c.t.Helper()
	var got string
	c.waitFor("welcome", func() bool {
		c.mu.Lock()
		got = c.playerID
		c.mu.Unlock()
		return got != ""
	})
	return got
}

// snapshot 取出目前看到的牌局狀態。
func (c *liveClient) snapshot() *testView {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.match
}

// roomSnapshot 取出目前看到的房間狀態。
func (c *liveClient) roomSnapshot() *roomView {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.room
}

// errorCount 回報至今收到幾則錯誤訊息。
func (c *liveClient) errorCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.errs)
}

// waitTimeout 是等待伺服器推播的上限。一局要打六十幾手，每手都是一次
// 往返，開了 -race 又會慢上好幾倍，所以這裡放得比單次往返寬鬆很多 ——
// 它是用來擋住「真的卡住了」，不是用來量效能。
const waitTimeout = 30 * time.Second

// waitFor 等待某個條件成立，逾時即讓測試失敗。
func (c *liveClient) waitFor(what string, cond func() bool) {
	c.t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.t.Fatalf("等待「%s」逾時（%s）", what, waitTimeout)
}

// TestEndToEndFullGame 走完最接近真人的完整流程：四條連線各自在背景收推播，
// 取名、開房、加入、開始，然後一路打到分出前三名。每一步都照伺服器推播的
// 狀態決定下一手，不預設任何牌局走向。
//
// 這是整個專案的整合測試：規則、房間、連線、視圖任何一層壞掉都會在這裡失敗。
