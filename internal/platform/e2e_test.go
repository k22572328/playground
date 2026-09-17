package platform

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestEndToEndPlatformFlow 走完最接近真人的平台流程：四條連線各自在背景
// 收推播，取名、開房、加入、開始，然後每個人輪流做一次動作直到結束。
//
// 這裡刻意用假遊戲 —— 驗證的是平台的流程（連線、房間、輪次推播、結束），
// 真正的大老二規則在 internal/games/bigtwo 有自己的端對端測試。
func TestEndToEndPlatformFlow(t *testing.T) {
	playFullGame(t, newTestServer(t))
}

// playFullGame 讓四個客戶端連線、開房、開局，然後一路做到結束。
func playFullGame(t *testing.T, ts *httptest.Server) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	players := make([]*liveClient, 4)
	names := []string{"房長", "客人1", "客人2", "客人3"}
	for i := range players {
		players[i] = dialLive(t, url)
		players[i].id()
		players[i].send(inbound{Action: actSetName, Name: names[i]})
	}

	host := players[0]
	host.send(inbound{Action: actCreateRoom, Name: "決戰房", KindID: testKindID})
	host.waitFor("房間建立", func() bool { return host.roomSnapshot() != nil })
	roomID := host.roomSnapshot().ID

	for _, p := range players[1:] {
		p.send(inbound{Action: actJoinRoom, RoomID: roomID})
	}
	host.waitFor("四人到齊", func() bool {
		r := host.roomSnapshot()
		return r != nil && len(r.Seats) == 4
	})

	host.send(inbound{Action: actStart})
	for i, p := range players {
		p.waitFor("收到牌局狀態", func() bool { return p.snapshot() != nil })
		if p.snapshot() == nil {
			t.Fatalf("玩家 %d 沒收到牌局狀態", i)
		}
	}

	// 每個人輪流做一次動作，做滿四次這局就結束。
	deadline := time.Now().Add(waitTimeout)
	for range 20 {
		if time.Now().After(deadline) {
			t.Fatalf("超過 %s 仍未結束，流程卡住了", waitTimeout)
		}
		actor := currentActor(players)
		if actor < 0 {
			if allFinished(players) {
				break
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		p := players[actor]
		before := p.snapshot()
		p.send(inbound{Action: actMove, Move: json.RawMessage(`{"note":"ok"}`)})
		p.waitFor("動作生效", func() bool { return p.snapshot() != before })
	}

	// 四個人都該看到結束。
	for i, p := range players {
		p.waitFor("看到結束", func() bool {
			v := p.snapshot()
			return v != nil && v.Over
		})
		if !p.snapshot().Over {
			t.Errorf("玩家 %d 沒看到牌局結束", i)
		}
	}
}

// allFinished 回報是否四個人都已經看到整局結束。
func allFinished(players []*liveClient) bool {
	for _, p := range players {
		if v := p.snapshot(); v == nil || !v.Over {
			return false
		}
	}
	return true
}

// currentActor 找出目前輪到誰動作；結束或視角還沒同步就回傳 -1。
//
// 每個客戶端各自收推播，某一瞬間看到的輪次可能不一致 —— 剛動完的人
// 已經更新，其他人還停在上一個畫面。必須等大家看到同一個輪次才動手，
// 否則會挑到一個早就過了回合的玩家而卡住。
func currentActor(players []*liveClient) int {
	turn := -1
	for _, p := range players {
		v := p.snapshot()
		if v == nil || v.Over {
			return -1
		}
		if turn < 0 {
			turn = v.Turn
		} else if turn != v.Turn {
			return -1
		}
	}
	for i, p := range players {
		if v := p.snapshot(); v != nil && v.YourSeat == turn {
			return i
		}
	}
	return -1
}
