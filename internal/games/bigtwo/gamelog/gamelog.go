// Package gamelog 把一局牌的過程寫成人看得懂的紀錄檔，供日後追查問題。
//
// 紀錄的內容足以重現整局：四家的初始手牌、每一手成功的出牌與 PASS、
// Round 的開始與結束、以及名次產生的時機。被規則擋下的嘗試不會記錄。
package gamelog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"playground/internal/games/bigtwo/match"
	"playground/internal/games/bigtwo/rules"
)

// Recorder 把一局牌的事件寫進一個紀錄檔，滿足 match.Observer。
//
// 它可以安全地被多個 goroutine 呼叫；寫檔失敗只會讓紀錄停止，
// 不會影響遊戲進行 —— 記錄失敗不該害玩家玩不下去。
type Recorder struct {
	mu      sync.Mutex
	file    *os.File
	names   [match.NumPlayers]string
	started time.Time

	// round 是目前進行到第幾個 Round，從 1 開始。
	round int

	// turn 是這局至今的第幾手，用來在每行前面標號。
	turn int

	// broken 記住寫檔已經失敗過，之後就不再嘗試也不再重複報錯。
	broken bool
}

// Create 開一個新的紀錄檔。檔名包含時間與房號，方便事後對照，
// 例如 logs/2026-09-17_143022_ABC.log。
//
// 即使建立失敗也會回傳一個可用的 Recorder（只是不寫東西），
// 讓呼叫端不必為了記錄失敗而中斷遊戲；err 供呼叫端記錄用。
func Create(dir, roomID string, names []string) (*Recorder, error) {
	now := time.Now()
	r := &Recorder{started: now, round: 1}
	copy(r.names[:], names)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.broken = true
		return r, fmt.Errorf("建立紀錄目錄 %s: %w", dir, err)
	}

	name := fmt.Sprintf("%s_%s.log", now.Format("2006-01-02_150405"), safe(roomID))
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		r.broken = true
		return r, fmt.Errorf("建立紀錄檔: %w", err)
	}
	r.file = f
	return r, nil
}

// Path 回報紀錄檔的位置；沒有實際寫檔時回傳空字串。
func (r *Recorder) Path() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return ""
	}
	return r.file.Name()
}

// Close 關閉紀錄檔。
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

// write 寫一行紀錄。寫檔出錯就停止記錄，但不影響遊戲。
func (r *Recorder) write(format string, args ...any) {
	if r.file == nil || r.broken {
		return
	}
	line := fmt.Sprintf(format, args...)
	if _, err := fmt.Fprintln(r.file, line); err != nil {
		r.broken = true
	}
}

// who 把座位寫成「座位2 阿明」這種好讀的樣子。
func (r *Recorder) who(seat int) string {
	if seat < 0 || seat >= match.NumPlayers {
		return fmt.Sprintf("座位%d", seat)
	}
	return fmt.Sprintf("座位%d %s", seat, r.names[seat])
}

// stamp 是每一行開頭的時間，用開局至今的秒數，方便看出節奏。
func (r *Recorder) stamp() string {
	return fmt.Sprintf("[%7.2fs]", time.Since(r.started).Seconds())
}

// Dealt 記錄發牌結果與誰先出。
func (r *Recorder) Dealt(hands [match.NumPlayers][]rules.Card, first int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.write("=== 大老二牌局紀錄 ===")
	r.write("開始時間：%s", r.started.Format("2006-01-02 15:04:05"))
	r.write("")
	r.write("--- 初始手牌 ---")
	for seat, h := range hands {
		r.write("%s：%s", r.who(seat), cards(h))
	}
	r.write("")
	r.write("%s 持有 ♣3，由他開局", r.who(first))
	r.write("")
	r.write("--- 第 1 個 Round ---")
}

// Played 記錄一次成功的出牌。
func (r *Recorder) Played(seat int, combo rules.Combo, rank int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.turn++
	r.write("%s 第%2d手  %s 出 %s：%s",
		r.stamp(), r.turn, r.who(seat), combo.Type, cards(combo.Cards))

	// 名次在打完最後一手的當下就確定，不必等其他人 PASS。
	if rank != 0 {
		r.write("%s        >>> %s 手牌出完，取得第 %d 名 <<<", r.stamp(), r.who(seat), rank)
	}
}

// Passed 記錄一次 PASS。
func (r *Recorder) Passed(seat int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.turn++
	r.write("%s 第%2d手  %s PASS（本 Round 起失去出牌資格）",
		r.stamp(), r.turn, r.who(seat))
}

// RoundEnded 記錄一個 Round 的結束與下一個 Round 的首攻者。
func (r *Recorder) RoundEnded(winner, starter int, bySuccession bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.write("%s        --- 第 %d 個 Round 結束，%s 贏下檯面 ---",
		r.stamp(), r.round, r.who(winner))
	if bySuccession {
		r.write("%s        %s 已離場，首攻權由順位交給 %s",
			r.stamp(), r.who(winner), r.who(starter))
	}

	r.round++
	r.write("")
	r.write("--- 第 %d 個 Round：%s 自由出牌 ---", r.round, r.who(starter))
}

// Finished 記錄整局的最終名次。
func (r *Recorder) Finished(ranks []int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.write("")
	r.write("=== 本局結束（共 %d 手，歷時 %s）===",
		r.turn, time.Since(r.started).Round(time.Second))
	for i, seat := range ranks {
		note := ""
		// 第四名是唯一沒把牌打完的人 —— 第三名產生時整局就結束了。
		if i == len(ranks)-1 {
			note = "（手牌未出完）"
		}
		r.write("第 %d 名：%s%s", i+1, r.who(seat), note)
	}
}

// cards 把一組牌寫成「3♣ 8♦ K♠」這種形式。
func cards(cs []rules.Card) string {
	if len(cs) == 0 {
		return "（無）"
	}
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = c.String()
	}
	return strings.Join(parts, " ")
}

// safe 去掉檔名裡不該出現的字元，避免房號帶進路徑分隔符號。
func safe(s string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, s)
	if clean == "" {
		return "room"
	}
	return clean
}
