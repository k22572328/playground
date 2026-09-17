package server

import (
	"sync"

	"bigTwo/internal/lobby"
)

// client 一條連線。每位玩家一條，斷線即移除。
type client struct {
	playerID string
	name     string

	// roomID 是玩家目前所在的房間；空字串代表他還在大廳。
	roomID string

	// send 是送給這條連線的訊息佇列，讀取端是這條連線自己的寫迴圈。
	// sendMu 同時保護「關閉」與「送出」兩件事：少了它，一邊在 close
	// 另一邊在 send 就會 panic。closed 記錄是否已經關過。
	sendMu sync.Mutex
	send   chan outbound
	closed bool
}

// close 關掉這條連線的送信佇列，讓寫迴圈結束。重複呼叫是安全的。
func (c *client) close() {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.send)
	}
}

// deliver 把訊息放進這條連線的佇列。佇列滿代表對方收得太慢，
// 直接關掉它；連線已關則直接丟棄。回報訊息是否送進佇列。
func (c *client) deliver(msg outbound) bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- msg:
		return true
	default:
		// 這裡已經持有 sendMu，所以直接關閉而不呼叫 close()。
		c.closed = true
		close(c.send)
		return false
	}
}

// hub 管理所有連線與它們所在的房間，是唯一會同時碰到 lobby 與連線的地方。
type hub struct {
	mu      sync.RWMutex
	clients map[string]*client // playerID -> 連線
	lobby   *lobby.Lobby
}

func newHub(l *lobby.Lobby) *hub {
	return &hub{clients: make(map[string]*client), lobby: l}
}

// add 登記一條新連線。同一個 playerID 重複連線時，舊的會被踢掉，
// 避免同一人開兩個分頁造成狀態分歧。
func (h *hub) add(c *client) {
	h.mu.Lock()
	old, exists := h.clients[c.playerID]
	h.clients[c.playerID] = c
	h.mu.Unlock()

	if exists {
		old.close()
	}
}

// remove 移除一條連線，並把它帶離所在的房間。
func (h *hub) remove(c *client) {
	h.mu.Lock()
	// 只有當這個 playerID 仍對應到同一條連線時才刪，
	// 否則會誤刪後來建立的新連線。
	if cur, ok := h.clients[c.playerID]; ok && cur == c {
		delete(h.clients, c.playerID)
	}
	roomID := c.roomID
	h.mu.Unlock()

	c.close()
	if roomID != "" {
		// 離開房間失敗多半是房間已被刪除，對斷線流程來說不算錯誤。
		_ = h.lobby.Leave(roomID, c.playerID)
		h.broadcastRoom(roomID)
		h.broadcastLobby()
	}
}

// setRoom 記錄某條連線目前所在的房間。
func (h *hub) setRoom(playerID, roomID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.clients[playerID]; ok {
		c.roomID = roomID
	}
}

// roomOf 回報某位玩家目前所在的房間；空字串代表他在大廳。
func (h *hub) roomOf(playerID string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if c, ok := h.clients[playerID]; ok {
		return c.roomID
	}
	return ""
}

// setName 設定某位玩家的暱稱。
func (h *hub) setName(playerID, name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.clients[playerID]; ok {
		c.name = name
	}
}

// nameOf 回報某位玩家的暱稱；還沒設定時為空字串。
func (h *hub) nameOf(playerID string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if c, ok := h.clients[playerID]; ok {
		return c.name
	}
	return ""
}

// send 把訊息送給單一玩家。
func (h *hub) send(playerID string, msg outbound) {
	h.mu.RLock()
	c, ok := h.clients[playerID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	c.deliver(msg)
}

// membersOf 列出目前人在某房間的所有連線。
func (h *hub) membersOf(roomID string) []*client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var out []*client
	for _, c := range h.clients {
		if c.roomID == roomID {
			out = append(out, c)
		}
	}
	return out
}

// lobbyWatchers 列出目前人在大廳的連線。
//
// 「在大廳」需要同時滿足兩件事：不在任何房間裡，而且已經取好暱稱。
// 少了後者，剛連上、還停在首頁輸入名字的人也會收到大廳推播，
// 前端一收到就切畫面，結果整桌人被別人取名字的動作一起拖進大廳。
func (h *hub) lobbyWatchers() []*client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var out []*client
	for _, c := range h.clients {
		if c.roomID == "" && c.name != "" {
			out = append(out, c)
		}
	}
	return out
}

// broadcastLobby 把最新的房間列表推給所有還在大廳的人。
//
// 每個人看到的列表略有不同（例如哪間房是自己開的），所以先在 Lobby 的鎖內
// 一次把所有人的畫面都算好，再於鎖外送出；送信可能被阻塞，不該佔著鎖。
func (h *hub) broadcastLobby() {
	watchers := h.lobbyWatchers()
	if len(watchers) == 0 {
		return
	}

	views := make([][]roomView, len(watchers))
	h.lobby.ReadAll(func(r *lobby.Room) {
		for i, c := range watchers {
			views[i] = append(views[i], newRoomView(r, c.playerID))
		}
	})

	for i, c := range watchers {
		c.deliver(outbound{Type: msgLobby, Rooms: views[i]})
	}
}

// broadcastRoom 把房間或牌局的最新狀態推給房裡的每個人。
// 每個人收到的內容不同 —— 手牌只送給本人。
func (h *hub) broadcastRoom(roomID string) {
	members := h.membersOf(roomID)
	if len(members) == 0 {
		return
	}

	msgs := make([]outbound, len(members))
	err := h.lobby.Read(roomID, func(r *lobby.Room) {
		for i, c := range members {
			msgs[i] = outbound{Type: msgRoom, Room: ptr(newRoomView(r, c.playerID))}
			if r.Started {
				msgs[i].Match = newMatchView(r.Match, r.SeatOf(c.playerID))
			}
		}
	})
	if err != nil {
		// 房間已消失（最後一人離開），沒有對象要通知。
		return
	}

	for i, c := range members {
		c.deliver(msgs[i])
	}
}

func ptr[T any](v T) *T { return &v }
