// Package server 把大廳與牌局接上 WebSocket，並提供前端靜態檔。
// 它是唯一認識 HTTP 的一層；規則與房間邏輯都在 game、match、lobby。
package server

import (
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"bigTwo/internal/game"
	"bigTwo/internal/gamelog"
	"bigTwo/internal/lobby"
	"bigTwo/internal/match"
	"bigTwo/internal/shuffle"
	"bigTwo/web"
)

const (
	// 一則訊息的大小上限；正常的動作遠小於此，過大視為惡意輸入。
	maxMessageBytes = 4 << 10

	// 送信佇列長度。塞滿代表對方收得太慢，直接斷線。
	sendQueueSize = 16

	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = pongWait * 9 / 10
)

var errEmptyName = errors.New("請先輸入暱稱")

// Server 提供遊戲的 HTTP 與 WebSocket 服務。
type Server struct {
	hub      *hub
	upgrader websocket.Upgrader
	webFS    fs.FS

	// nextPlayer 產生玩家識別碼。
	nextPlayer atomic.Int64
}

// Options 調整伺服器的行為。零值即為合理的預設：內嵌前端、不寫牌局紀錄。
type Options struct {
	// WebFS 是前端靜態檔的來源；nil 表示使用編進執行檔的版本。
	WebFS fs.FS

	// LogDir 是牌局紀錄的存放目錄；空字串表示不記錄。
	LogDir string
}

// New 建立一台伺服器，前端頁面取自內嵌的靜態檔，不寫牌局紀錄。
func New() *Server { return NewWithOptions(Options{}) }

// NewWithFS 建立一台伺服器，並指定前端靜態檔的來源。
// 開發時可以傳入 os.DirFS("web")，改了前端不必重新編譯。
func NewWithFS(webFS fs.FS) *Server { return NewWithOptions(Options{WebFS: webFS}) }

// NewWithOptions 依 opts 建立一台伺服器。
func NewWithOptions(opts Options) *Server {
	webFS := opts.WebFS
	if webFS == nil {
		webFS = web.FS()
	}

	// 用 crypto/rand 洗牌：玩家不該能從先前的牌局推算出後續的發牌，
	// 而且它沒有共用狀態，多個房間同時發牌也安全。
	var shuffler shuffle.Crypto
	l := lobby.New(shuffler)
	if opts.LogDir != "" {
		l = lobby.NewWithRecorder(shuffler, recorderFactory(opts.LogDir))
	}

	return &Server{
		hub:   newHub(l),
		webFS: webFS,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// 這是區網內自架的遊戲，不做跨站來源限制。
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// recorderFactory 產生一個在 dir 底下開紀錄檔的工廠。
// 開檔失敗只記在伺服器日誌裡，遊戲照常進行 —— 記錄不該擋住玩家。
func recorderFactory(dir string) lobby.NewRecorder {
	return func(roomID string, names [match.NumPlayers]string) lobby.Recorder {
		rec, err := gamelog.Create(dir, roomID, names)
		if err != nil {
			log.Printf("建立牌局紀錄失敗（遊戲繼續進行）: %v", err)
			return rec
		}
		log.Printf("房間 %s 開局，紀錄寫入 %s", roomID, rec.Path())
		return rec
	}
}

// Handler 組出完整的路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.Handle("/", http.FileServerFS(s.webFS))
	return mux
}

// handleWS 把一條 HTTP 連線升級成 WebSocket，並開始服務這位玩家。
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade 失敗時已自行回覆過錯誤，這裡只記錄。
		log.Printf("websocket 升級失敗: %v", err)
		return
	}

	c := &client{
		playerID: s.newPlayerID(),
		send:     make(chan outbound, sendQueueSize),
	}
	// welcome 必須是這條連線收到的第一則訊息，所以搶在 hub.add 之前放進佇列：
	// 一旦登記進 hub，其他 goroutine 的廣播就可能先擠進來。
	c.deliver(outbound{Type: msgWelcome, PlayerID: c.playerID})
	s.hub.add(c)

	go s.writeLoop(conn, c)
	s.readLoop(conn, c)
}

// newPlayerID 產生一組不重複的玩家識別碼。
func (s *Server) newPlayerID() string {
	return "p" + itoa(s.nextPlayer.Add(1))
}

// readLoop 持續讀取這條連線送來的動作，直到斷線。
func (s *Server) readLoop(conn *websocket.Conn, c *client) {
	defer func() {
		s.hub.remove(c)
		conn.Close()
	}()

	conn.SetReadLimit(maxMessageBytes)
	// 收到 pong 就延長期限；對方沒回應就會在 pongWait 後讀取逾時。
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		var msg inbound
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("讀取失敗 (%s): %v", c.playerID, err)
			}
			return
		}
		s.dispatch(c, msg)
	}
}

// writeLoop 把佇列裡的訊息送出去，並定期 ping 確認連線還活著。
func (s *Server) writeLoop(conn *websocket.Conn, c *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				// hub 關掉了佇列，代表這條連線該收工了。
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					time.Now().Add(writeWait))
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				return
			}
		}
	}
}

// dispatch 執行一個前端送來的動作。
func (s *Server) dispatch(c *client, msg inbound) {
	if err := s.perform(c, msg); err != nil {
		s.hub.send(c.playerID, errorMsg(err))
	}
}

// perform 依動作型別做事，回傳的錯誤會直接顯示給該玩家。
func (s *Server) perform(c *client, msg inbound) error {
	switch msg.Action {
	case actSetName:
		return s.setName(c, msg.Name)
	case actCreateRoom:
		return s.createRoom(c, msg.Name)
	case actJoinRoom:
		return s.joinRoom(c, msg.RoomID)
	case actLeaveRoom:
		return s.leaveRoom(c)
	case actKick:
		return s.kick(c, msg.TargetID)
	case actStart:
		return s.start(c)
	case actPlay:
		return s.play(c, msg.Cards)
	case actPass:
		return s.play(c, nil)
	default:
		return errors.New("不認得的動作: " + msg.Action)
	}
}

// setName 設定暱稱，設定完就能看到大廳。
func (s *Server) setName(c *client, name string) error {
	name = cleanName(name)
	if name == "" {
		return errEmptyName
	}
	s.hub.setName(c.playerID, name)
	s.hub.broadcastLobby()
	return nil
}

// createRoom 建立房間並直接進去當房長。
func (s *Server) createRoom(c *client, roomName string) error {
	name := s.hub.nameOf(c.playerID)
	if name == "" {
		return errEmptyName
	}
	roomName = cleanName(roomName)
	if roomName == "" {
		roomName = name + " 的房間"
	}

	r := s.hub.lobby.Create(roomName, lobby.Seat{PlayerID: c.playerID, Name: name})
	s.hub.setRoom(c.playerID, r.ID)
	s.hub.broadcastRoom(r.ID)
	s.hub.broadcastLobby()
	return nil
}

// joinRoom 加入指定房間。
func (s *Server) joinRoom(c *client, roomID string) error {
	name := s.hub.nameOf(c.playerID)
	if name == "" {
		return errEmptyName
	}

	r, err := s.hub.lobby.Join(roomID, lobby.Seat{PlayerID: c.playerID, Name: name})
	if err != nil {
		return err
	}
	s.hub.setRoom(c.playerID, r.ID)
	s.hub.broadcastRoom(r.ID)
	s.hub.broadcastLobby()
	return nil
}

// leaveRoom 離開目前的房間，回到大廳。
func (s *Server) leaveRoom(c *client) error {
	roomID := s.hub.roomOf(c.playerID)
	if roomID == "" {
		return lobby.ErrNotInRoom
	}
	if err := s.hub.lobby.Leave(roomID, c.playerID); err != nil {
		return err
	}
	s.hub.setRoom(c.playerID, "")
	s.hub.broadcastRoom(roomID)
	s.hub.broadcastLobby()
	return nil
}

// kick 由房長把某人踢出房間。
func (s *Server) kick(c *client, targetID string) error {
	roomID := s.hub.roomOf(c.playerID)
	if roomID == "" {
		return lobby.ErrNotInRoom
	}
	if err := s.hub.lobby.Kick(roomID, c.playerID, targetID); err != nil {
		return err
	}

	// 被踢的人回到大廳，並收到一則說明。
	s.hub.setRoom(targetID, "")
	s.hub.send(targetID, outbound{Type: msgKicked, Message: "你被房長請出房間了"})

	s.hub.broadcastRoom(roomID)
	s.hub.broadcastLobby()
	return nil
}

// start 由房長開始遊戲，需要滿四人。
func (s *Server) start(c *client) error {
	roomID := s.hub.roomOf(c.playerID)
	if roomID == "" {
		return lobby.ErrNotInRoom
	}
	if _, err := s.hub.lobby.Start(roomID, c.playerID); err != nil {
		return err
	}
	s.hub.broadcastRoom(roomID)
	s.hub.broadcastLobby() // 房間狀態變成遊戲中，大廳列表也要更新
	return nil
}

// play 出牌；cards 為空代表 PASS。
func (s *Server) play(c *client, refs []cardRef) error {
	roomID := s.hub.roomOf(c.playerID)
	if roomID == "" {
		return lobby.ErrNotInRoom
	}
	cards, err := toCards(refs)
	if err != nil {
		return err
	}
	if err := s.hub.lobby.PlayInRoom(roomID, c.playerID, cards); err != nil {
		return err
	}
	s.hub.broadcastRoom(roomID)
	return nil
}

// toCards 把前端送來的牌轉成引擎用的型別，順便擋掉超出範圍的數值。
func toCards(refs []cardRef) ([]game.Card, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	cards := make([]game.Card, 0, len(refs))
	for _, ref := range refs {
		if ref.Rank < int(game.Three) || ref.Rank > int(game.Two) {
			return nil, game.ErrInvalidCombo
		}
		if ref.Suit < int(game.Clubs) || ref.Suit > int(game.Spades) {
			return nil, game.ErrInvalidCombo
		}
		cards = append(cards, game.Card{Rank: game.Rank(ref.Rank), Suit: game.Suit(ref.Suit)})
	}
	return cards, nil
}

// cleanName 去除前後空白並限制長度，避免版面被超長暱稱撐破。
func cleanName(s string) string {
	s = strings.TrimSpace(s)
	const maxRunes = 12
	if r := []rune(s); len(r) > maxRunes {
		s = string(r[:maxRunes])
	}
	return s
}

// itoa 是給 playerID 用的小工具，避免為了一個數字引入 strconv。
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
