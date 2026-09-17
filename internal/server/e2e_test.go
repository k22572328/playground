package server

import (
	"strings"
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
	match    *matchView
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
			if msg.Match != nil {
				c.match = msg.Match
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
func (c *liveClient) snapshot() *matchView {
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

// waitFor 等待某個條件成立，逾時即讓測試失敗。
func (c *liveClient) waitFor(what string, cond func() bool) {
	c.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.t.Fatalf("等待「%s」逾時", what)
}

// TestEndToEndFullGame 走完最接近真人的完整流程：四條連線各自在背景收推播，
// 取名、開房、加入、開始，然後一路打到分出前三名。每一步都照伺服器推播的
// 狀態決定下一手，不預設任何牌局走向。
//
// 這是整個專案的整合測試：規則、房間、連線、視圖任何一層壞掉都會在這裡失敗。
func TestEndToEndFullGame(t *testing.T) {
	ts := newTestServer(t)
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// 四位玩家連線並取名。
	players := make([]*liveClient, 4)
	names := []string{"房長", "客人1", "客人2", "客人3"}
	for i := range players {
		players[i] = dialLive(t, url)
		players[i].id() // 等 welcome
		players[i].send(inbound{Action: actSetName, Name: names[i]})
	}

	// 房長開房，其餘三人加入。
	host := players[0]
	host.send(inbound{Action: actCreateRoom, Name: "決戰房"})
	host.waitFor("房間建立", func() bool { return host.roomSnapshot() != nil })
	roomID := host.roomSnapshot().ID

	for _, p := range players[1:] {
		p.send(inbound{Action: actJoinRoom, RoomID: roomID})
	}
	host.waitFor("四人到齊", func() bool {
		r := host.roomSnapshot()
		return r != nil && len(r.Seats) == 4
	})

	// 開始遊戲，等每個人都拿到手牌。
	host.send(inbound{Action: actStart})
	for i, p := range players {
		p.waitFor("收到手牌", func() bool {
			m := p.snapshot()
			return m != nil && len(m.YourHand) == 13
		})
		if got := len(p.snapshot().YourHand); got != 13 {
			t.Fatalf("玩家 %d 手牌 %d 張，應該 13 張", i, got)
		}
	}

	// 一路打到結束。每一輪找出當前出牌者，試著出牌，不行就 PASS。
	const maxTurns = 600
	turns := 0
	for ; turns < maxTurns; turns++ {
		actor := currentActor(players)
		if actor < 0 {
			break // 沒有人能動，代表已經結束
		}
		p := players[actor]
		before := p.snapshot()
		if before.Over {
			break
		}

		if !tryPlaySomething(t, p, before) {
			// 出不了就 PASS。
			if !before.CanPass {
				t.Fatalf("玩家 %d 既不能出牌也不能 PASS，流程卡住", actor)
			}
			p.send(inbound{Action: actPass})
			p.waitFor("PASS 生效", func() bool { return p.snapshot() != before })
		}
	}

	if turns >= maxTurns {
		t.Fatalf("跑了 %d 手仍未結束，流程可能卡住", maxTurns)
	}

	// 等所有人都看到結束狀態。
	for i, p := range players {
		p.waitFor("看到結算", func() bool {
			m := p.snapshot()
			return m != nil && m.Over
		})
		m := p.snapshot()

		if len(m.Rankings) != 3 {
			t.Errorf("玩家 %d 看到 %d 位名次，應該是 3 位", i, len(m.Rankings))
		}
		for j, r := range m.Rankings {
			if r.Rank != j+1 {
				t.Errorf("玩家 %d 看到第 %d 位的名次是 %d", i, j+1, r.Rank)
			}
		}
		// 恰好一位沒有名次，就是第四名。
		unranked := 0
		for _, s := range m.Seats {
			if s.Rank == 0 {
				unranked++
			}
		}
		if unranked != 1 {
			t.Errorf("玩家 %d 看到 %d 位沒排上名次，應該剛好 1 位", i, unranked)
		}
	}

	// 四個人看到的名次必須完全一致。
	want := players[0].snapshot().Rankings
	for i, p := range players[1:] {
		got := p.snapshot().Rankings
		for j := range want {
			if got[j].Seat != want[j].Seat {
				t.Errorf("玩家 %d 與房長看到的第 %d 名不同：座位 %d vs %d",
					i+1, j+1, got[j].Seat, want[j].Seat)
			}
		}
	}
}

// currentActor 找出目前輪到誰出牌；沒有人輪到就回傳 -1。
func currentActor(players []*liveClient) int {
	for i, p := range players {
		if m := p.snapshot(); m != nil && m.CanPlay && !m.Over {
			return i
		}
	}
	return -1
}

// tryPlaySomething 嘗試從手牌裡找出一組能打出去的牌並送出，成功回報 true。
// 依序試單張與對子 —— 這足以讓任何局面推進下去。
func tryPlaySomething(t *testing.T, p *liveClient, before *matchView) bool {
	t.Helper()

	var attempts [][]cardRef
	for _, c := range before.YourHand {
		attempts = append(attempts, []cardRef{{Rank: c.Rank, Suit: c.Suit}})
	}
	// 對子：相鄰且同點數的兩張。
	for i := 1; i < len(before.YourHand); i++ {
		a, b := before.YourHand[i-1], before.YourHand[i]
		if a.Rank == b.Rank {
			attempts = append(attempts, []cardRef{
				{Rank: a.Rank, Suit: a.Suit}, {Rank: b.Rank, Suit: b.Suit},
			})
		}
	}

	for _, cards := range attempts {
		errsBefore := p.errorCount()
		p.send(inbound{Action: actPlay, Cards: cards})

		// 這一手要嘛被接受（狀態改變），要嘛被拒絕（多一則錯誤）。
		var accepted bool
		p.waitFor("出牌有回應", func() bool {
			if p.snapshot() != before {
				accepted = true
				return true
			}
			return p.errorCount() > errsBefore
		})
		if accepted {
			return true
		}
	}
	return false
}
