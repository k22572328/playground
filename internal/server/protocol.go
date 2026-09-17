package server

// 前端與伺服器之間的訊息格式。前端送 inbound，伺服器回 outbound。

// 前端送過來的動作。
const (
	actSetName    = "setName"    // 輸入暱稱，進入大廳
	actCreateRoom = "createRoom" // 建立房間
	actJoinRoom   = "joinRoom"   // 加入房間
	actLeaveRoom  = "leaveRoom"  // 離開房間
	actKick       = "kick"       // 房長踢人
	actStart      = "start"      // 房長開始遊戲
	actPlay       = "play"       // 出牌
	actPass       = "pass"       // PASS
	actResume     = "resume"     // 帶著 token 接回先前的身分
)

// 伺服器推播的訊息類型。
const (
	msgWelcome = "welcome" // 連線建立，附上這條連線的玩家識別
	msgLobby   = "lobby"   // 大廳房間列表
	msgRoom    = "room"    // 房間或牌局狀態
	msgKicked  = "kicked"  // 你被房長踢出房間了
	msgError   = "error"   // 動作失敗的原因
)

// inbound 前端送來的一則訊息。
type inbound struct {
	Action string `json:"action"`

	Name     string `json:"name"`     // setName / createRoom 用
	RoomID   string `json:"roomId"`   // joinRoom 用
	TargetID string `json:"targetId"` // kick 用

	// Token 是上次連線拿到的身分識別，重新整理或斷線重連時帶回來，
	// 讓伺服器認出這是同一個人。
	Token string `json:"token"`

	// Cards 是出牌時選的牌，用 rank/suit 的數值表示。
	Cards []cardRef `json:"cards"`
}

// cardRef 前端指定一張牌的方式。
type cardRef struct {
	Rank int `json:"rank"`
	Suit int `json:"suit"`
}

// outbound 伺服器送出的一則訊息。沒用到的欄位會被省略，
// 讓前端能用同一個處理函式分辨訊息內容。
type outbound struct {
	Type string `json:"type"`

	PlayerID string     `json:"playerId,omitempty"` // welcome 用
	Token    string     `json:"token,omitempty"`    // welcome 用：存起來供重連
	Rooms    []roomView `json:"rooms,omitempty"`    // lobby 用
	Room     *roomView  `json:"room,omitempty"`     // room 用
	Match    *matchView `json:"match,omitempty"`    // room 用，遊戲開始後才有
	Message  string     `json:"message,omitempty"`  // error / kicked 用
}

// errorMsg 包裝一則錯誤訊息。
func errorMsg(err error) outbound {
	return outbound{Type: msgError, Message: err.Error()}
}
