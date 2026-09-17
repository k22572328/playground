package platform

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// 平台的測試需要「某一種遊戲」才能開房、開局、出手，但平台本身不該
// 認識任何特定遊戲（真正的遊戲反而是 import 平台的，直接用會造成循環依賴）。
//
// 所以這裡放一個極簡的假遊戲：四個人輪流做動作，每人做滿一次就結束。
// 它只是用來驅動平台的流程，規則本身沒有意義 ——
// 真正的大老二規則在 internal/games/bigtwo 有自己的測試。

const testKindID = "testgame"

// 測試共用一組註冊表，所以只註冊一次。
var registerOnce sync.Once

func registerTestGame() {
	registerOnce.Do(func() {
		Register(Kind{
			ID:       testKindID,
			Name:     "測試遊戲",
			MinSeats: 4,
			MaxSeats: 4,
			New: func(names []string, s Shuffler, rec Recorder) Instance {
				return &testInstance{names: names, rec: rec}
			},
			NewRecorder: func(dir, roomID string, names []string) (Recorder, error) {
				return newTestRecorder(dir, roomID)
			},
		})
	})
}

// testInstance 是那個假遊戲的一局。
type testInstance struct {
	names []string

	// turn 是輪到誰動作；acted 記錄已經動過幾次。
	turn  int
	acted int

	rec Recorder
}

// testMove 是假遊戲唯一的動作內容。
type testMove struct {
	Note string `json:"note"`
}

var errNotYourTurn = errors.New("還沒輪到你")

func (g *testInstance) Act(seat int, raw json.RawMessage) error {
	if g.Over() {
		return errors.New("這局已經結束了")
	}
	if seat != g.turn {
		return errNotYourTurn
	}
	// 內容只要是合法 JSON 就好；這裡解一次是為了驗證平台有把它原樣轉交。
	if len(raw) > 0 {
		var m testMove
		if err := json.Unmarshal(raw, &m); err != nil {
			return errors.New("看不懂的動作內容")
		}
	}
	g.acted++
	g.turn = (g.turn + 1) % len(g.names)
	return nil
}

func (g *testInstance) Over() bool { return g.acted >= len(g.names) }

// testView 是假遊戲送給前端的狀態，形狀刻意模仿真實遊戲：
// 有「輪到我了嗎」以及斷線資訊，平台的測試靠這些欄位判斷流程。
type testView struct {
	YourSeat   int      `json:"yourSeat"`
	Turn       int      `json:"turn"`
	CanAct     bool     `json:"canAct"`
	Over       bool     `json:"over"`
	Offline    []int    `json:"offline,omitempty"`
	WaitingFor []string `json:"waitingFor,omitempty"`
}

func (g *testInstance) ViewFor(seat int, offline []Offline) any {
	v := &testView{
		YourSeat: seat,
		Turn:     g.turn,
		CanAct:   !g.Over() && seat == g.turn,
		Over:     g.Over(),
	}
	for _, off := range offline {
		v.Offline = append(v.Offline, off.Seat)
		v.WaitingFor = append(v.WaitingFor, off.Name)
	}
	// 有人斷線時牌局暫停，動作按鈕該停掉。
	if len(v.WaitingFor) > 0 {
		v.CanAct = false
	}
	return v
}

// testRecorder 是假遊戲的紀錄檔。真實遊戲會寫詳細的過程，
// 這裡只要證明「平台確實有建立檔案並在結束時關閉」就夠了。
type testRecorder struct {
	f *os.File
}

func newTestRecorder(dir, roomID string) (Recorder, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, roomID+".log"))
	if err != nil {
		return nil, err
	}
	_, _ = f.WriteString("測試遊戲紀錄\n")
	return &testRecorder{f: f}, nil
}

func (r *testRecorder) Close() error {
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}
