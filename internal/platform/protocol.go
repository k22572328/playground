package platform

import "encoding/json"

// 前端與伺服器之間的訊息格式。前端送 inbound，伺服器回 outbound。

// 前端送過來的動作。
const (
	actSetName    = "setName"    // 輸入暱稱，進入大廳
	actCreateRoom = "createRoom" // 建立房間
	actJoinRoom   = "joinRoom"   // 加入房間
	actLeaveRoom  = "leaveRoom"  // 離開房間
	actKick       = "kick"       // 房長踢人
	actStart      = "start"      // 房長開始遊戲
	actMove       = "move"       // 遊戲內的動作（出牌、擲骰…）
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

	// KindID 是建立房間時選的遊戲種類。
	KindID string `json:"kindId"`

	// Move 是遊戲內的動作，內容由該遊戲自己解析 ——
	// 大老二是要出的牌，別的遊戲可能是擲骰或買地。
	Move json.RawMessage `json:"move"`
}

// outbound 伺服器送出的一則訊息。沒用到的欄位會被省略，
// 讓前端能用同一個處理函式分辨訊息內容。
type outbound struct {
	Type string `json:"type"`

	PlayerID string     `json:"playerId,omitempty"` // welcome 用
	Token    string     `json:"token,omitempty"`    // welcome 用：存起來供重連
	Games    []gameInfo `json:"games,omitempty"`    // welcome 用：可以開哪些遊戲
	Rooms    []roomView `json:"rooms,omitempty"`    // lobby 用
	Room     *roomView  `json:"room,omitempty"`     // room 用
	Game     any        `json:"game,omitempty"`     // room 用，遊戲開始後才有；內容由該遊戲決定
	Message  string     `json:"message,omitempty"`  // error / kicked 用
}

// errorMsg 包裝一則錯誤訊息。
func errorMsg(err error) outbound {
	return outbound{Type: msgError, Message: err.Error()}
}

// gameInfo 告訴前端有哪些遊戲可以開，以及各自需要幾個人。
type gameInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MinSeats int    `json:"minSeats"`
	MaxSeats int    `json:"maxSeats"`
}

// gameInfos 把已註冊的遊戲整理成前端要的形狀。
func gameInfos() []gameInfo {
	ks := Kinds()
	out := make([]gameInfo, 0, len(ks))
	for _, k := range ks {
		out = append(out, gameInfo{
			ID: k.ID, Name: k.Name, MinSeats: k.MinSeats, MaxSeats: k.MaxSeats,
		})
	}
	return out
}
