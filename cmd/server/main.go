// Command server 啟動大老二的遊戲伺服器。
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"playground/internal/platform"
)

func main() {
	addr := flag.String("addr", ":8080", "監聽位址")
	webRoot := flag.String("web", "",
		"前端靜態檔目錄；留空則使用編進執行檔的版本")
	logDir := flag.String("logdir", "logs",
		"牌局紀錄的存放目錄；設為空字串則不記錄")
	flag.Parse()

	// 預設用內嵌的前端，所以在任何目錄下執行都能正常供應頁面。
	// 指定 -web 才改讀外部目錄，方便開發時改了前端立刻看到效果。
	srv := &http.Server{
		Addr:    *addr,
		Handler: newServer(*webRoot, *logDir).Handler(),
		// WebSocket 連線會長時間開著，所以不設整體寫入逾時，
		// 改由連線層自己的 ping/pong 與寫入期限把關。
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("大老二伺服器啟動： http://localhost%s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("伺服器結束: %v", err)
	}
}

// newServer 依參數組出伺服器。指定了外部前端目錄就先確認它真的存在，
// 否則寧可啟動失敗，也不要默默跑出一台每頁都 404 的伺服器。
func newServer(webRoot, logDir string) *platform.Server {
	opts := platform.Options{LogDir: logDir}

	if webRoot != "" {
		if _, err := os.Stat(filepath.Join(webRoot, "index.html")); err != nil {
			log.Fatalf("找不到前端檔案 %s/index.html：%v\n"+
				"（拿掉 -web 參數就會改用編進執行檔的前端）", webRoot, err)
		}
		log.Printf("前端改用外部目錄：%s", webRoot)
		opts.WebFS = os.DirFS(webRoot)
	}

	if logDir == "" {
		log.Print("未指定 -logdir，本次不寫牌局紀錄")
	} else {
		if abs, err := filepath.Abs(logDir); err == nil {
			logDir = abs
		}
		log.Printf("牌局紀錄將寫入：%s", logDir)
	}
	return platform.NewWithOptions(opts)
}
