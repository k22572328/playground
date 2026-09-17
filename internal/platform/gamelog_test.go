package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGameLogWritten 驗證平台會替每一局開一份紀錄檔。
//
// 內容格式是各遊戲自己的事（大老二的格式在它自己的測試裡驗），
// 平台只負責建立檔案並在結束時關閉。
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
	if !strings.HasSuffix(entries[0].Name(), ".log") {
		t.Errorf("紀錄檔副檔名應為 .log，實際 %q", entries[0].Name())
	}

	raw, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("讀取紀錄檔失敗: %v", err)
	}
	if len(raw) == 0 {
		t.Error("紀錄檔不該是空的")
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
