package platform

import "encoding/json"

// 這個檔案定義平台與各種遊戲之間的界線。
//
// 平台負責的是所有遊戲都一樣的事：連線、登入、大廳、開房、座位、
// 斷線重連。它完全不知道房間裡在玩什麼 —— 那是 Kind 與 Instance 的事。
//
// 要加一種新遊戲，就實作這兩個介面並註冊進來，平台不必改。

// Shuffler 把 n 個元素洗成隨機排列，用於發牌或決定先後順序。
// 抽成介面是為了讓測試能給定固定序列。
type Shuffler interface {
	Shuffle(n int, swap func(i, j int))
}

// Recorder 記錄一局的過程，供日後追查問題。
// 內容格式由各遊戲自己決定，平台只負責把它交給遊戲並在結束時關閉。
type Recorder interface {
	Close() error
}

// Kind 描述一種遊戲：它叫什麼、需要幾個人、怎麼開一局。
type Kind struct {
	// ID 是程式用的識別碼，例如 "bigtwo"。前端靠它決定要載入哪個遊戲畫面。
	ID string

	// Name 是顯示給玩家看的名稱，例如「大老二」。
	Name string

	// MinSeats 與 MaxSeats 是這個遊戲能開局的人數範圍。
	// 大老二兩者都是 4；其他遊戲可能允許一個區間。
	MinSeats int
	MaxSeats int

	// New 開始一局。names 依座位順序給出玩家名稱，長度保證在
	// [MinSeats, MaxSeats] 之間。rec 可能為 nil（不記錄這局）。
	New func(names []string, s Shuffler, rec Recorder) Instance

	// NewRecorder 替一局開一份紀錄檔，內容格式由該遊戲自己決定。
	// 為 nil 表示這個遊戲不做紀錄；回傳的 Recorder 會原封不動交還給 New。
	// 開檔失敗時仍該回傳一個可用（但不寫入）的 Recorder，讓遊戲照常進行。
	NewRecorder func(dir, roomID string, names []string) (Recorder, error)
}

// SeatsOK 回報這個人數能不能開局。
func (k Kind) SeatsOK(n int) bool { return n >= k.MinSeats && n <= k.MaxSeats }

// Offline 描述一位斷線中的玩家，讓遊戲把狀態標進自己的畫面。
type Offline struct {
	Seat int
	Name string
}

// Instance 是一局進行中的遊戲。平台只透過這個介面操作它，
// 不知道裡面是在出牌、擲骰還是走棋。
type Instance interface {
	// Act 讓某個座位做一個動作。action 的內容由各遊戲自己定義與解析 ——
	// 大老二傳的是要出的牌，別的遊戲可能傳擲骰或買地。
	// 回傳的錯誤會直接顯示給那位玩家，所以訊息要寫得讓人看得懂。
	Act(seat int, action json.RawMessage) error

	// ViewFor 回傳 seat 這個座位看得到的狀態，會被編成 JSON 送給前端。
	// 實作必須確保玩家只看得到自己該看的（例如別人的手牌只給張數）。
	// seat 為 -1 表示旁觀者。
	//
	// offline 列出目前斷線中的座位。斷線是平台的概念，遊戲本身不管連線，
	// 但畫面要標示出來，而且輪到斷線者時該停掉動作按鈕 —— 牌局是暫停的。
	ViewFor(seat int, offline []Offline) any

	// Over 回報這局是否已經結束。
	Over() bool
}

// registry 是已知的遊戲種類。平台本身不 import 任何遊戲套件
// （那會造成循環依賴），而是由 cmd/server 在啟動時註冊進來。
var registry = map[string]Kind{}

// Register 登記一種遊戲。重複註冊同一個 ID 會 panic —— 那是程式設定錯誤，
// 早點爆掉比之後發現兩種遊戲互相覆蓋好。
func Register(k Kind) {
	if k.ID == "" {
		panic("platform: 遊戲的 ID 不能是空的")
	}
	if _, dup := registry[k.ID]; dup {
		panic("platform: 重複註冊遊戲 " + k.ID)
	}
	if k.MinSeats < 1 || k.MaxSeats < k.MinSeats {
		panic("platform: 遊戲 " + k.ID + " 的人數範圍不合理")
	}
	if k.New == nil {
		panic("platform: 遊戲 " + k.ID + " 沒有提供 New")
	}
	registry[k.ID] = k
}

// KindByID 取出一種已註冊的遊戲。
func KindByID(id string) (Kind, bool) {
	k, ok := registry[id]
	return k, ok
}

// Kinds 列出所有已註冊的遊戲，供大廳顯示可以開哪些房。
// 順序依 ID 排序，讓選單每次都一樣。
func Kinds() []Kind {
	out := make([]Kind, 0, len(registry))
	for _, k := range registry {
		out = append(out, k)
	}
	sortKinds(out)
	return out
}

// sortKinds 依 ID 排序，避免 map 的走訪順序讓選單跳來跳去。
func sortKinds(ks []Kind) {
	for i := 1; i < len(ks); i++ {
		for j := i; j > 0 && ks[j].ID < ks[j-1].ID; j-- {
			ks[j], ks[j-1] = ks[j-1], ks[j]
		}
	}
}
