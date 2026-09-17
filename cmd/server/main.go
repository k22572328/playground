// Command server 啟動大老二的遊戲伺服器。
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"bigTwo/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "監聽位址")
	webRoot := flag.String("web", "web", "前端靜態檔目錄")
	flag.Parse()

	srv := &http.Server{
		Addr:    *addr,
		Handler: server.New(*webRoot).Handler(),
		// WebSocket 連線會長時間開著，所以不設整體寫入逾時，
		// 改由連線層自己的 ping/pong 與寫入期限把關。
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("大老二伺服器啟動： http://localhost%s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("伺服器結束: %v", err)
	}
}
