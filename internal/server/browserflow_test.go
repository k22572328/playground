package server

import (
	"strings"
	"testing"
	"time"
)

// TestBrowserRefreshFlow 模擬瀏覽器實際的行為：
// 前端每次連上都會拿到新 token，但重連時送的是「上一次存起來的舊 token」。
// 這個測試照著那個順序走一遍，確認真實流程可行。
func TestBrowserRefreshFlow(t *testing.T) {
	ts := newTestServer(t)

	// --- 第一次開啟頁面 ---
	tab := dial(t, ts)
	stored := tab.token // 前端存進 localStorage 的就是這個
	tab.setName("阿明")

	// 開一間房，並找個人一起待著。
	tab.send(inbound{Action: actCreateRoom, Name: "重整測試房"})
	before := tab.readUntil(msgRoom)
	roomID := before.Room.ID
	joinRoom(t, ts, "室友", roomID)
	tab.readUntil(msgRoom)

	// --- 使用者按下重新整理：舊連線斷掉，新頁面連上 ---
	tab.conn.Close()
	time.Sleep(100 * time.Millisecond)

	fresh := dial(t, ts)
	// 前端這時手上有兩個 token：新連線發的，以及 localStorage 裡的舊的。
	// 它會用舊的去接回身分 —— 正是這裡要驗證的。
	if fresh.token == stored {
		t.Fatal("每次連線應該發新的 token")
	}
	fresh.send(inbound{Action: actResume, Token: stored})

	// 遊戲還沒開始就斷線的話，座位不會保留（留著只會擋住別人加入），
	// 所以重整後回到大廳 —— 但暱稱還在，不必重新輸入。
	after := fresh.readUntil(msgLobby)
	seen := false
	for _, r := range after.Rooms {
		if r.ID == roomID {
			seen = true
		}
	}
	if !seen {
		t.Error("原本那間房還有人，應該仍出現在大廳列表")
	}

	// 暱稱保留著：開新房時會用它當預設房名。
	fresh.send(inbound{Action: actCreateRoom, Name: ""})
	msg := fresh.readUntil(msgRoom)
	if !strings.Contains(msg.Room.Name, "阿明") {
		t.Errorf("重整後應保留暱稱，房名 = %q", msg.Room.Name)
	}
}
