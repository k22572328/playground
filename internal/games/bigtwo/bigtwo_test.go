package bigtwo

import (
	"encoding/json"
	"testing"

	"playground/internal/games/bigtwo/rules"
	"playground/internal/platform"
	"playground/internal/shuffle"
)

// newTestGame 開一局大老二，回傳平台看到的介面與底層的牌局。
func newTestGame(t *testing.T) *instance {
	t.Helper()
	g := newInstance([]string{"A", "B", "C", "D"}, shuffle.Crypto{}, nil)
	in, ok := g.(*instance)
	if !ok {
		t.Fatalf("newInstance 回傳了非預期的型別 %T", g)
	}
	return in
}

// moveJSON 把幾張牌包成前端會送出的動作內容。
func moveJSON(t *testing.T, cards ...rules.Card) json.RawMessage {
	t.Helper()
	refs := make([]cardRef, len(cards))
	for i, c := range cards {
		refs[i] = cardRef{Rank: int(c.Rank), Suit: int(c.Suit)}
	}
	raw, err := json.Marshal(action{Cards: refs})
	if err != nil {
		t.Fatalf("編碼動作失敗: %v", err)
	}
	return raw
}

// TestKindIsWellFormed 驗證註冊資訊正確 —— 平台靠它決定人數與顯示名稱。
func TestKindIsWellFormed(t *testing.T) {
	k := Kind()

	if k.ID != ID {
		t.Errorf("ID = %q，應該是 %q", k.ID, ID)
	}
	if k.Name == "" {
		t.Error("應該要有顯示給玩家看的名稱")
	}
	if k.MinSeats != 4 || k.MaxSeats != 4 {
		t.Errorf("大老二固定四人，實際 %d~%d", k.MinSeats, k.MaxSeats)
	}
	if !k.SeatsOK(4) {
		t.Error("四人應該可以開局")
	}
	for _, n := range []int{0, 1, 3, 5} {
		if k.SeatsOK(n) {
			t.Errorf("%d 人不該能開局", n)
		}
	}
	if k.New == nil || k.NewRecorder == nil {
		t.Error("Kind 應該同時提供 New 與 NewRecorder")
	}
}

// TestActPlaysCards 驗證動作能正確解析成出牌：開局者打出梅花 3。
func TestActPlaysCards(t *testing.T) {
	in := newTestGame(t)

	first := in.m.Turn
	if err := in.Act(first, moveJSON(t, rules.ClubThree)); err != nil {
		t.Fatalf("開局出梅花 3 應該成功: %v", err)
	}
	if in.m.Turn == first {
		t.Error("出牌後應該換下一個人")
	}
	if in.Over() {
		t.Error("才出一張牌，這局不該結束")
	}
}

// TestActRejectsBadInput 驗證畸形或超出範圍的動作會被擋下，
// 而不是讓伺服器出錯 —— 這些內容是從網路來的，不能信任。
func TestActRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"不是 JSON", `{壞掉的`},
		{"cards 型別不對", `{"cards":"不是陣列"}`},
		{"負數點數", `{"cards":[{"rank":-1,"suit":0}]}`},
		{"超大點數", `{"cards":[{"rank":99,"suit":0}]}`},
		{"負數花色", `{"cards":[{"rank":0,"suit":-3}]}`},
		{"超大花色", `{"cards":[{"rank":0,"suit":99}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := newTestGame(t)
			if err := in.Act(in.m.Turn, json.RawMessage(tt.raw)); err == nil {
				t.Error("畸形的動作內容應該被擋下")
			}
		})
	}
}

// TestViewHidesOtherHands 驗證最重要的一條：你只看得到自己的手牌，
// 別人一律只揭露張數。
func TestViewHidesOtherHands(t *testing.T) {
	in := newTestGame(t)

	for seat := range 4 {
		v, ok := in.ViewFor(seat, nil).(*matchView)
		if !ok {
			t.Fatalf("ViewFor 回傳了非預期的型別")
		}
		if len(v.YourHand) != 13 {
			t.Errorf("座位 %d 應看到自己的 13 張手牌，實際 %d", seat, len(v.YourHand))
		}
		if v.YourSeat != seat {
			t.Errorf("YourSeat = %d，應該是 %d", v.YourSeat, seat)
		}
		// 四個座位都只揭露張數，沒有任何一張別人的牌洩漏出去。
		for _, s := range v.Seats {
			if s.CardCount != 13 {
				t.Errorf("座位 %d 應顯示 13 張，實際 %d", s.Seat, s.CardCount)
			}
		}
	}
}

// TestViewMarksOffline 驗證斷線資訊會被標進牌桌畫面，並停掉出牌按鈕 ——
// 有人斷線時牌局是暫停的。
func TestViewMarksOffline(t *testing.T) {
	in := newTestGame(t)
	turn := in.m.Turn

	// 沒人斷線時，輪到的那位應該可以出牌。
	v := in.ViewFor(turn, nil).(*matchView)
	if !v.CanPlay {
		t.Error("沒人斷線時，輪到的玩家應該能出牌")
	}

	// 有人斷線時，即使輪到你也不能動。
	offline := []platform.Offline{{Seat: (turn + 1) % 4, Name: "離線的人"}}
	v = in.ViewFor(turn, offline).(*matchView)
	if v.CanPlay || v.CanPass {
		t.Error("有人斷線時牌局應暫停，不能出牌或 PASS")
	}
	if len(v.WaitingFor) != 1 || v.WaitingFor[0] != "離線的人" {
		t.Errorf("應該列出在等誰，實際 %v", v.WaitingFor)
	}
	marked := 0
	for _, s := range v.Seats {
		if s.Offline {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("應有 1 個座位標記為斷線，實際 %d", marked)
	}
}

// TestFullGameThroughInterface 只透過平台的介面打完一整局，
// 確認 Act 與 Over 足以驅動整個牌局到結束。
func TestFullGameThroughInterface(t *testing.T) {
	in := newTestGame(t)

	const maxTurns = 400
	for turn := 0; !in.Over(); turn++ {
		if turn >= maxTurns {
			t.Fatalf("跑了 %d 手仍未結束，流程可能卡住", maxTurns)
		}

		seat := in.m.Turn
		moves := in.m.LegalMoves(seat)
		if len(moves) == 0 {
			// 出不了就 PASS，用空的動作內容表示。
			if err := in.Act(seat, moveJSON(t)); err != nil {
				t.Fatalf("座位 %d PASS 失敗: %v", seat, err)
			}
			continue
		}
		if err := in.Act(seat, moveJSON(t, moves[0].Cards...)); err != nil {
			t.Fatalf("座位 %d 出牌失敗: %v", seat, err)
		}
	}

	// 結束後應該排出前三名，第四名不必把牌打完。
	v := in.ViewFor(0, nil).(*matchView)
	if !v.Over {
		t.Error("牌局應該結束了")
	}
	if len(v.Rankings) != 3 {
		t.Errorf("應排出 3 位名次，實際 %d", len(v.Rankings))
	}
}
