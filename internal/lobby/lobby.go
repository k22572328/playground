// Package lobby 管理房間的生命週期：建立、加入、離開、踢人與開始遊戲。
// 它只認識 match 層，不碰任何 HTTP 或連線細節。
package lobby

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"bigTwo/internal/game"
	"bigTwo/internal/match"
)

var (
	ErrRoomNotFound   = errors.New("房間不存在")
	ErrRoomFull       = errors.New("房間已滿")
	ErrRoomStarted    = errors.New("遊戲已經開始")
	ErrNotHost        = errors.New("只有房長能做這件事")
	ErrNotEnough      = errors.New("人數不足，無法開始")
	ErrAlreadyJoined  = errors.New("你已經在這個房間了")
	ErrNotInRoom      = errors.New("你不在這個房間")
	ErrCannotKickSelf = errors.New("房長不能踢自己")
	ErrNotStarted     = errors.New("遊戲還沒開始")
)

// Seat 房間裡的一個位子。
type Seat struct {
	PlayerID string `json:"playerId"`
	Name     string `json:"name"`
}

// Room 一個房間。房長是座位 0 的人；房長離開時由下一位遞補。
type Room struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Seats   []Seat `json:"seats"`
	Started bool   `json:"started"`

	// Match 在遊戲開始後才存在。
	Match *match.Match `json:"-"`

	CreatedAt time.Time `json:"createdAt"`
}

// Host 回報房長；空房間沒有房長。
func (r *Room) Host() (Seat, bool) {
	if len(r.Seats) == 0 {
		return Seat{}, false
	}
	return r.Seats[0], true
}

// IsHost 回報 playerID 是否為房長。
func (r *Room) IsHost(playerID string) bool {
	host, ok := r.Host()
	return ok && host.PlayerID == playerID
}

// Full 回報房間是否已滿。
func (r *Room) Full() bool { return len(r.Seats) >= match.NumPlayers }

// SeatOf 回報 playerID 在牌局中的座位；不在房裡則回傳 -1。
// 座位順序就是房間裡的排列順序，開始遊戲後不再變動。
func (r *Room) SeatOf(playerID string) int { return r.indexOf(playerID) }

// indexOf 找出 playerID 在房裡的座位序，不在則回傳 -1。
func (r *Room) indexOf(playerID string) int {
	for i, s := range r.Seats {
		if s.PlayerID == playerID {
			return i
		}
	}
	return -1
}

// Lobby 是所有房間的集合，可安全地並行存取。
type Lobby struct {
	mu     sync.RWMutex
	rooms  map[string]*Room
	nextID int

	// rng 供發牌使用；集中在這裡以便測試注入固定亂數。
	rng *rand.Rand
}

// New 建立一個空的大廳。
func New(rng *rand.Rand) *Lobby {
	return &Lobby{rooms: make(map[string]*Room), rng: rng}
}

// List 列出目前所有房間，供大廳畫面顯示。
//
// 回傳的是共用的 *Room，只有在持有 Lobby 的鎖時讀取才安全；
// 要在鎖外使用請改用 ReadAll，或用 Read 取用單一房間。
func (l *Lobby) List() []*Room {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]*Room, 0, len(l.rooms))
	for _, r := range l.rooms {
		out = append(out, r)
	}
	return out
}

// Get 取出一個房間。與 List 相同，回傳的指標只在持有鎖時讀取才安全。
func (l *Lobby) Get(roomID string) (*Room, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return nil, ErrRoomNotFound
	}
	return r, nil
}

// Read 在持有讀鎖的情況下把房間交給 fn 使用，讓呼叫端能安全地讀取房間狀態。
// fn 只能讀，不能保留 r 或在裡面呼叫其他 Lobby 方法（會造成重複上鎖）。
func (l *Lobby) Read(roomID string, fn func(r *Room)) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return ErrRoomNotFound
	}
	fn(r)
	return nil
}

// ReadAll 在持有讀鎖的情況下依序把每個房間交給 fn，用於產生大廳列表。
// 與 Read 相同，fn 只能讀，不能保留指標或回頭呼叫 Lobby。
func (l *Lobby) ReadAll(fn func(r *Room)) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, r := range l.rooms {
		fn(r)
	}
}

// Create 建立房間，建立者自動成為房長並坐進座位 0。
func (l *Lobby) Create(name string, host Seat) *Room {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.nextID++
	r := &Room{
		ID:        roomID(l.nextID),
		Name:      name,
		Seats:     []Seat{host},
		CreatedAt: time.Now(),
	}
	l.rooms[r.ID] = r
	return r
}

// Join 讓玩家加入房間。
func (l *Lobby) Join(roomID string, p Seat) (*Room, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return nil, ErrRoomNotFound
	}
	if r.Started {
		return nil, ErrRoomStarted
	}
	if r.indexOf(p.PlayerID) >= 0 {
		return nil, ErrAlreadyJoined
	}
	if r.Full() {
		return nil, ErrRoomFull
	}
	r.Seats = append(r.Seats, p)
	return r, nil
}

// Leave 讓玩家離開房間。房長離開時由下一位遞補；房間空了就刪除。
func (l *Lobby) Leave(roomID, playerID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return ErrRoomNotFound
	}
	i := r.indexOf(playerID)
	if i < 0 {
		return ErrNotInRoom
	}
	// 座位 0 一定是房長，所以移掉後由原本的下一位自然遞補。
	r.Seats = append(r.Seats[:i], r.Seats[i+1:]...)
	if len(r.Seats) == 0 {
		delete(l.rooms, roomID)
	}
	return nil
}

// Kick 由房長把某位玩家踢出房間。遊戲開始後不能踢人。
func (l *Lobby) Kick(roomID, hostID, targetID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return ErrRoomNotFound
	}
	if !r.IsHost(hostID) {
		return ErrNotHost
	}
	if r.Started {
		return ErrRoomStarted
	}
	if hostID == targetID {
		return ErrCannotKickSelf
	}
	i := r.indexOf(targetID)
	if i < 0 {
		return ErrNotInRoom
	}
	r.Seats = append(r.Seats[:i], r.Seats[i+1:]...)
	return nil
}

// Start 由房長開始遊戲，需要坐滿四人。
func (l *Lobby) Start(roomID, hostID string) (*match.Match, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return nil, ErrRoomNotFound
	}
	if !r.IsHost(hostID) {
		return nil, ErrNotHost
	}
	if r.Started {
		return nil, ErrRoomStarted
	}
	if !r.Full() {
		return nil, ErrNotEnough
	}

	var names [match.NumPlayers]string
	for i, s := range r.Seats {
		names[i] = s.Name
	}
	r.Match = match.New(names, l.rng)
	r.Started = true
	return r.Match, nil
}

// PlayInRoom 在房間的牌局裡替 playerID 出牌；cards 為空代表 PASS。
// 出牌會改動牌局狀態，所以和其他讀取一樣要在 Lobby 的鎖內進行。
func (l *Lobby) PlayInRoom(roomID, playerID string, cards []game.Card) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return ErrRoomNotFound
	}
	if !r.Started || r.Match == nil {
		return ErrNotStarted
	}
	seat := r.indexOf(playerID)
	if seat < 0 {
		return ErrNotInRoom
	}
	return r.Match.Play(seat, cards)
}

// roomID 產生簡短好記的房號。
func roomID(n int) string {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // 去掉容易誤認的 I 與 O
	id := []byte{
		letters[(n/24/24)%24],
		letters[(n/24)%24],
		letters[n%24],
	}
	return string(id)
}
