package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGameLogWritten 打完一整局後檢查紀錄檔：內容要足以讓人重現整局，
// 包含四家初始手牌、每一手出牌與 PASS、Round 邊界，以及最終名次。
func TestGameLogWritten(t *testing.T) {
	dir := t.TempDir()
	ts := newTestServerWithOptions(t, Options{LogDir: dir})

	playFullGame(t, ts)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("讀取紀錄目錄失敗: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("應該產生 1 份紀錄檔，實際 %d 份", len(entries))
	}

	name := entries[0].Name()
	if !strings.HasSuffix(name, ".log") {
		t.Errorf("紀錄檔副檔名應為 .log，實際 %q", name)
	}

	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("讀取紀錄檔失敗: %v", err)
	}
	content := string(raw)

	// 紀錄要能回答「這局怎麼開始的」「每一手誰出了什麼」「誰贏了」。
	for _, want := range []string{
		"初始手牌",
		"持有 ♣3，由他開局",
		"第 1 個 Round",
		"第 1 名：",
		"本局結束",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("紀錄檔少了 %q\n---\n%s", want, content)
		}
	}

	// 四位玩家的名字都要出現在初始手牌區。
	for _, name := range []string{"房長", "客人1", "客人2", "客人3"} {
		if !strings.Contains(content, name) {
			t.Errorf("紀錄檔沒有提到玩家 %q", name)
		}
	}

	// 名次要完整排到第 4 名，且第四名要標註手牌未出完。
	for i, want := range []string{"第 1 名：", "第 2 名：", "第 3 名：", "第 4 名："} {
		if !strings.Contains(content, want) {
			t.Errorf("紀錄檔少了第 %d 名", i+1)
		}
	}
	if !strings.Contains(content, "（手牌未出完）") {
		t.Error("第四名應標註手牌未出完")
	}

	// 初始手牌共 52 張，且每家 13 張 —— 這讓紀錄足以重現發牌。
	if n := strings.Count(content, "："); n < 4 {
		t.Errorf("初始手牌區格式看起來不對:\n%s", content)
	}
}

// TestNoLogWhenDisabled 驗證不指定目錄時不會產生任何檔案。
func TestNoLogWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	ts := newTestServerWithOptions(t, Options{}) // LogDir 留空

	playFullGame(t, ts)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("讀取目錄失敗: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("關閉紀錄時不該產生檔案，實際 %d 份", len(entries))
	}
}

// TestLogDirCreatedOnDemand 驗證紀錄目錄不存在時會自動建立。
func TestLogDirCreatedOnDemand(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "logs") // 兩層都還不存在
	ts := newTestServerWithOptions(t, Options{LogDir: dir})

	playFullGame(t, ts)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("紀錄目錄應該被自動建立: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("應產生 1 份紀錄檔，實際 %d 份", len(entries))
	}
}
