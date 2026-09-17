package platform

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// 這個檔案處理「重新整理後還是同一個人」這件事。
//
// 身分本來完全綁在 WebSocket 連線上，連線一斷就什麼都沒了 ——
// 重新整理頁面就變成新玩家，回不去進行中的牌局。
//
// 作法是伺服器發一組隨機 token 給瀏覽器存進 localStorage，
// 重連時帶回來，伺服器認得就把舊身分接回去。

const (
	// reconnectGrace 是斷線後替玩家保留座位的時間。
	// 太短會讓短暫的網路不穩就出局，太長則讓其他三人空等。
	reconnectGrace = 60 * time.Second

	// sessionTTL 是 token 在完全沒有使用的情況下保留多久。
	// 比寬限期長一些，讓「剛好在寬限期邊緣回來」的情況仍然認得出來。
	sessionTTL = 10 * time.Minute
)

// session 是一個玩家跨連線的身分。
type session struct {
	token string

	// playerID 是這個身分目前對應的連線識別。每次重連都會換一組新的，
	// 因為 hub 是用 playerID 當鍵值來管理連線的。
	playerID string

	name   string
	roomID string

	// expires 是這個 session 的失效時間；每次活動都會往後延。
	expires time.Time

	// dropAt 非零表示玩家正在斷線中，到這個時間還沒回來就真的移出房間。
	// 它同時用來分辨兩種「同一個身分出現第二條連線」的情況：
	// 非零代表原本那條已經斷了，這是重連；零代表原本那條還活著，
	// 這是別人拿著 token 來搶佔，必須擋下來保護先連上的人。
	dropAt time.Time

	// live 表示這個身分目前有一條活著的連線。
	live bool
}

// online 回報這個身分目前是否有連線活著 —— 用來擋下搶佔。
func (s *session) online() bool { return s.live && s.dropAt.IsZero() }

// sessions 管理所有玩家的跨連線身分，可安全地並行存取。
type sessions struct {
	mu     sync.Mutex
	byUUID map[string]*session // token -> session
}

func newSessions() *sessions {
	return &sessions{byUUID: make(map[string]*session)}
}

// create 產生一組新的身分。
func (s *sessions) create(playerID string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess := &session{
		token:    newToken(),
		playerID: playerID,
		expires:  time.Now().Add(sessionTTL),
		live:     true,
	}
	s.byUUID[sess.token] = sess
	return sess
}

// get 依 token 取出身分；找不到或已過期都回傳 nil。
func (s *sessions) get(token string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.byUUID[token]
	if !ok {
		return nil
	}
	if time.Now().After(sess.expires) {
		delete(s.byUUID, token)
		return nil
	}
	return sess
}

// update 在鎖的保護下修改一個身分。
func (s *sessions) update(token string, fn func(*session)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess, ok := s.byUUID[token]; ok {
		fn(sess)
		sess.expires = time.Now().Add(sessionTTL)
	}
}

// drop 移除一個身分。
func (s *sessions) drop(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byUUID, token)
}

// expiredDrops 找出寬限期已過、該真正移出房間的身分。
// 回傳的是複本，呼叫端可以在鎖外安全使用。
func (s *sessions) expiredDrops(now time.Time) []session {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []session
	for token, sess := range s.byUUID {
		if !sess.dropAt.IsZero() && now.After(sess.dropAt) {
			out = append(out, *sess)
			delete(s.byUUID, token)
			continue
		}
		// 順便清掉太久沒用的身分，避免無限累積。
		if now.After(sess.expires) {
			delete(s.byUUID, token)
		}
	}
	return out
}

// newToken 產生一組無法猜測的身分識別。用 crypto/rand 是因為
// 猜中別人的 token 就等於能接管他的座位與手牌。
func newToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 失敗代表系統的亂數來源壞了，
		// 這時發出可預測的 token 比直接停下來更危險。
		panic("server: 無法產生 session token: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
