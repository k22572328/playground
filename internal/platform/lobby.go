// Package lobby 管理房間的生命週期：建立、加入、離開、踢人與開始遊戲。
// 它只認識 match 層，不碰任何 HTTP 或連線細節。
package platform

import (
	"errors"
	"sync"
	"time"

	"playground/internal/games/bigtwo/match"
	"playground/internal/games/bigtwo/rules"
)

var (
	ErrRoomNotFound     = errors.New("房間不存在")
	ErrRoomFull         = errors.New("房間已滿")
	ErrRoomStarted      = errors.New("遊戲已經開始")
	ErrNotHost          = errors.New("只有房長能做這件事")
	ErrNotEnough        = errors.New("人數不足，無法開始")
	ErrAlreadyJoined    = errors.New("你已經在這個房間了")
	ErrNotInRoom        = errors.New("你不在這個房間")
	ErrCannotKickSelf   = errors.New("房長不能踢自己")
	ErrNotStarted       = errors.New("遊戲還沒開始")
	ErrWaitingForPlayer = errors.New("有玩家斷線中，牌局暫停")
)

// Seat 房間裡的一個位子。
type Seat struct {
	PlayerID string `json:"playerId"`
	Name     string `json:"name"`

	// Offline 表示這位玩家目前斷線中，座位替他保留著等他回來。
	// 只有在遊戲進行中才會發生：還沒開局的話斷線就直接離開房間。
	Offline bool `json:"offline"`
}

// Room 一個房間。房長由 hostID 指定，離開時交給剩下的第一位；
// 座位順序在開局時會重洗，所以不能用座位序來認定房長。
type Room struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Seats   []Seat `json:"seats"`
	Started bool   `json:"started"`

	// Match 在遊戲開始後才存在。
	Match *match.Match `json:"-"`

	// hostID 是房長。開局會重新洗座位，所以房長不能用「座位 0」來認定。
	hostID string

	// recorder 是這局的紀錄檔，遊戲結束或房間關閉時要收起來。
	recorder Recorder

	CreatedAt time.Time `json:"createdAt"`
}

// Host 回報房長；空房間沒有房長。
func (r *Room) Host() (Seat, bool) {
	if i := r.indexOf(r.hostID); i >= 0 {
		return r.Seats[i], true
	}
	return Seat{}, false
}

// IsHost 回報 playerID 是否為房長。
func (r *Room) IsHost(playerID string) bool {
	return playerID != "" && playerID == r.hostID
}

// Full 回報房間是否已滿。
func (r *Room) Full() bool { return len(r.Seats) >= match.NumPlayers }

// SeatOf 回報 playerID 在牌局中的座位；不在房裡則回傳 -1。
// 座位在開局時洗過一次，之後整局不再變動。
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

// Recorder 是一局牌的紀錄檔。它同時是 match 的觀察者，並且要能被關閉。
type Recorder interface {
	match.Observer
	Close() error
}

// NewRecorder 替一局牌開一份紀錄。回傳 nil 表示這局不做紀錄。
type NewRecorder func(roomID string, names [match.NumPlayers]string) Recorder

// Lobby 是所有房間的集合，可安全地並行存取。
type Lobby struct {
	mu     sync.RWMutex
	rooms  map[string]*Room
	nextID int

	// shuffler 供發牌與排座位使用；抽成介面是為了讓測試注入固定序列。
	shuffler match.Shuffler

	// newRecorder 在開局時建立紀錄檔；nil 表示不記錄。
	newRecorder NewRecorder
}

// New 建立一個空的大廳，不產生牌局紀錄。
func newLobby(s match.Shuffler) *Lobby {
	return &Lobby{rooms: make(map[string]*Room), shuffler: s}
}

// NewWithRecorder 建立一個會把每局牌寫成紀錄檔的大廳。
func newLobbyWithRecorder(s match.Shuffler, nr NewRecorder) *Lobby {
	l := newLobby(s)
	l.newRecorder = nr
	return l
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

// Create 建立房間，建立者自動成為房長。
func (l *Lobby) Create(name string, host Seat) *Room {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.nextID++
	r := &Room{
		ID:        roomID(l.nextID),
		Name:      name,
		Seats:     []Seat{host},
		hostID:    host.PlayerID,
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

// MarkOffline 把玩家標記為斷線中，替他保留座位。
// 只在遊戲進行中有意義；還沒開局時應該直接 Leave。
// 回傳這個標記是否真的造成改變，讓呼叫端決定要不要廣播。
func (l *Lobby) MarkOffline(roomID, playerID string) (changed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return false
	}
	i := r.indexOf(playerID)
	if i < 0 || r.Seats[i].Offline {
		return false
	}
	r.Seats[i].Offline = true
	return true
}

// Reconnect 讓斷線的玩家接回原本的座位，並換上新的連線識別。
//
// oldID 是他斷線前的身分，newID 是這條新連線的身分。成功回傳房間，
// 呼叫端接著要把完整的牌局狀態推回去給他。
func (l *Lobby) Reconnect(roomID, oldID, newID string) (*Room, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return nil, ErrRoomNotFound
	}
	i := r.indexOf(oldID)
	if i < 0 {
		return nil, ErrNotInRoom
	}

	r.Seats[i].PlayerID = newID
	r.Seats[i].Offline = false
	// 房長斷線又回來的話，房長身分也要跟著換到新的識別。
	if r.hostID == oldID {
		r.hostID = newID
	}
	return r, nil
}

// AnyoneOffline 回報房裡是否有人正在斷線中。
// 牌局在這種時候要暫停，免得輪到斷線者卻沒人能出牌。
func (r *Room) AnyoneOffline() bool {
	for _, s := range r.Seats {
		if s.Offline {
			return true
		}
	}
	return false
}

// OfflineNames 列出目前斷線中的玩家名字，用來告訴其他人在等誰。
func (r *Room) OfflineNames() []string {
	var names []string
	for _, s := range r.Seats {
		if s.Offline {
			names = append(names, s.Name)
		}
	}
	return names
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
	r.Seats = append(r.Seats[:i], r.Seats[i+1:]...)
	if len(r.Seats) == 0 {
		r.closeRecorder()
		delete(l.rooms, roomID)
		return nil
	}
	// 房長離開就把房長交給剩下的第一位。
	if playerID == r.hostID {
		r.hostID = r.Seats[0].PlayerID
	}
	return nil
}

// closeRecorder 收起這個房間的紀錄檔。重複呼叫是安全的。
func (r *Room) closeRecorder() {
	if r.recorder == nil {
		return
	}
	_ = r.recorder.Close()
	r.recorder = nil
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

	// 開局時重新洗座位，這樣誰坐哪不會由進房順序決定 ——
	// 房長也就沒有「先進來就固定坐在誰的上家」這種優勢。
	l.shuffler.Shuffle(len(r.Seats), func(i, j int) {
		r.Seats[i], r.Seats[j] = r.Seats[j], r.Seats[i]
	})

	var names [match.NumPlayers]string
	for i, s := range r.Seats {
		names[i] = s.Name
	}

	// 每開一局就開一份紀錄檔，日後追查問題用。記錄失敗不影響開局。
	var obs match.Observer
	if l.newRecorder != nil {
		r.recorder = l.newRecorder(roomID, names)
		obs = r.recorder
	}

	r.Match = match.NewWithObserver(names, l.shuffler, obs)
	r.Started = true
	return r.Match, nil
}

// PlayInRoom 在房間的牌局裡替 playerID 出牌；cards 為空代表 PASS。
// 出牌會改動牌局狀態，所以和其他讀取一樣要在 Lobby 的鎖內進行。
func (l *Lobby) PlayInRoom(roomID, playerID string, cards []rules.Card) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	r, ok := l.rooms[roomID]
	if !ok {
		return ErrRoomNotFound
	}
	if !r.Started || r.Match == nil {
		return ErrNotStarted
	}
	// 有人斷線時牌局暫停：否則輪到斷線者就沒人能出牌，
	// 而其他人繼續出下去，等他回來局面早就不是他離開時的樣子了。
	if r.AnyoneOffline() {
		return ErrWaitingForPlayer
	}
	seat := r.indexOf(playerID)
	if seat < 0 {
		return ErrNotInRoom
	}
	err := r.Match.Play(seat, cards)

	// 一局打完就把紀錄收起來，確保內容落到磁碟上，
	// 不必等房間解散。Finished 已經在 Play 裡寫進去了。
	if r.Match.Over() {
		r.closeRecorder()
	}
	return err
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
