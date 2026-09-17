# bigTwo 開發須知

台灣規則四人大老二。Go 後端 + 原生 HTML/CSS/JS 前端。

## 環境

**這個專案在 GOPATH 底下，但使用 Go modules。所有 go 指令都要加 `GO111MODULE=on`**，
否則會出現 `modules disabled by GO111MODULE=off`：

```bash
GO111MODULE=on go test ./...
GO111MODULE=on go run ./cmd/server
```

Go 1.23，所以不能用 1.24 才有的 `testing.B.Loop`。

## 分層原則

依賴單向往外，內層不知道外層存在：

```
game  ←  match  ←  lobby  ←  server  ←  cmd/server
```

- `internal/game`：純規則，無狀態、無 I/O。牌型判斷與比大小。
- `internal/match`：一局的流程。Round、PASS 資格、名次、順位。
- `internal/lobby`：房間管理。**所有會改動 Match 的操作都要走這一層**，
  因為它持有保護共用狀態的鎖。
- `internal/server`：WebSocket 與資料視圖。唯一認識 HTTP 的一層。

改規則只會動到 `game` 與 `match`。加功能前先想清楚該放哪一層。

## 併發規則

`lobby.Lobby` 的鎖保護所有房間與牌局狀態。**不要在鎖外讀取 `Room` 的欄位**
（包含 `Seats` 與 `Match`），要用 `Read` / `ReadAll` 把讀取包進鎖裡。
出牌一律走 `PlayInRoom`，不要自己拿 `r.Match` 出來呼叫。

送訊息給連線用 `client.deliver`，它同時處理「佇列已滿」與「連線已關」。
**不要直接對 `c.send` 做 channel 操作**。

改動 `server` 或 `lobby` 之後務必跑 `-race`，一般測試抓不到這類錯誤。

## 規則細節

完整規格見 README，最容易寫錯的幾點：

- 合法牌型只有六種，**三條與普通同花不能出牌**。
- 普通牌型之間**沒有跨牌型大小關係**，葫蘆與順子不能互壓。
- 鐵支與同花順可以無條件壓普通牌型，**不受張數限制**。
- 順子順位：`A2345` 最小、`23456` 最大，`JQKA2` 不成立。
- `23456` 用 **2** 當比較牌。
- PASS 之後該 Round 永久失去出牌資格，即使檯面被炸掉也一樣。

## 測試

日常開發用 `-short`（跳過十萬局隨機對局與窮舉測試），提交前跑完整版：

```bash
GO111MODULE=on go test -short ./...        # 快，日常用
GO111MODULE=on go test ./...               # 完整，約 2~4 分鐘
GO111MODULE=on go test -race -short ./...  # 改 server/lobby 後必跑
```

寫測試時請涵蓋臨界與極端情況，不要只測正常路徑 —— 這個專案的幾個真 bug
都是靠邊界測試與 `-race` 抓出來的。
