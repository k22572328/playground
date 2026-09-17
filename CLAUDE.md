# playground 開發須知

多人連線的遊戲平台，目前收錄台灣規則四人大老二。
Go 後端 + 原生 HTML/CSS/JS 前端，module 名稱是 `playground`。

## 環境

**這個專案在 GOPATH 底下，但使用 Go modules。所有 go 指令都要加 `GO111MODULE=on`**，
否則會出現 `modules disabled by GO111MODULE=off`：

```bash
GO111MODULE=on go test ./...
GO111MODULE=on go run ./cmd/server
```

Go 1.23，所以不能用 1.24 才有的 `testing.B.Loop`。

## 分層原則

最重要的一條界線是**平台與遊戲**：平台負責所有遊戲都一樣的事
（連線、登入、大廳、開房、座位、斷線重連），它**完全不知道房間裡在玩什麼**。

```
             internal/platform  ←  cmd/server  →  internal/games/bigtwo
             （通用，不認識任何遊戲）  組裝      ↑  rules ← match ← bigtwo
```

- `internal/platform`：大廳、房間、WebSocket、session、資料視圖。
  唯一認識 HTTP 的一層。**所有會改動牌局的操作都要走這一層**，
  因為它持有保護共用狀態的鎖。
- `internal/platform/game.go`：平台與遊戲之間的界線，定義 `Kind` 與 `Instance`。
  平台只透過 `Act(seat, json.RawMessage)` / `ViewFor` / `Over` 操作遊戲。
- `internal/games/bigtwo`：把大老二接上 `platform.Instance`，
  負責翻譯動作與整理畫面資料。底下三個子套件才是規則本身：
  - `rules`：純規則，無狀態、無 I/O。牌型判斷與比大小。
  - `match`：一局的流程。Round、PASS 資格、名次、順位。
  - `gamelog`：把一局牌寫成人看得懂的紀錄檔，實作 `match.Observer`。
- `internal/shuffle`：發牌與排座位用的洗牌器，平台與遊戲共用。
- `web`：前端靜態檔，並用 `go:embed` 把它們編進執行檔。
  `web/app.js` 是平台，各遊戲的畫面在 `web/games/<id>/table.js`。
  **改了 `web/` 底下的檔案要重新編譯才會生效**，開發時可用 `-web web` 繞過。

`platform` **不 import 任何遊戲套件**（那會造成循環依賴），
而是由 `cmd/server` 的 `init()` 呼叫 `platform.Register(bigtwo.Kind())` 組裝起來。

改大老二的規則只會動到 `rules` 與 `match`。加功能前先想清楚：
這件事是所有遊戲都要的（放 `platform`），還是只有大老二要的（放 `games/bigtwo`）。

## 加一個新遊戲

平台不必改。照這四步：

1. 在 `internal/games/<id>/` 實作 `platform.Instance`：
   `Act`（解析該遊戲自己定義的動作 JSON）、`ViewFor`（**只能回傳該座位看得到的東西**）、
   `Over`。再提供一個回傳 `platform.Kind` 的函式，填 ID、顯示名稱與人數範圍。
2. 在 `cmd/server/main.go` 的 `init()` 裡 `platform.Register(...)`。
3. 寫 `web/games/<id>/table.js`，用
   `Playground.registerGame(id, { screen, mount, render, unmount })` 掛進前端。
   `mount` 會拿到 `{ send, leaveRoom }`，`send(move)` 送出的東西就是後端 `Act` 收到的。
4. 在 `web/index.html` 加上該遊戲的畫面區塊與 `<script>`。

前端靠 `Kind.ID` 決定載入哪個畫面，所以兩邊的 id 字串必須一致。

## 併發規則

`platform.Lobby` 的鎖保護所有房間與牌局狀態。**不要在鎖外讀取 `Room` 的欄位**
（包含 `Seats` 與 `Game`），要用 `Read` / `ReadAll` 把讀取包進鎖裡。
玩家動作一律走 `ActInRoom`，不要自己拿 `r.Game` 出來呼叫。

送訊息給連線用 `client.deliver`，它同時處理「佇列已滿」與「連線已關」。
**不要直接對 `c.send` 做 channel 操作**。

改動 `internal/platform` 之後務必跑 `-race`，一般測試抓不到這類錯誤。

## 規則細節

完整規格見 README，最容易寫錯的幾點：

- 合法牌型只有六種，**三條與普通同花不能出牌**。
- 普通牌型之間**沒有跨牌型大小關係**，葫蘆與順子不能互壓。
- 鐵支與同花順可以無條件壓普通牌型，**不受張數限制**。
- 順子順位：`A2345` 最小、`23456` 最大，`JQKA2` 不成立。
- `23456` 用 **2** 當比較牌。
- PASS 之後該 Round 永久失去出牌資格，即使檯面被炸掉也一樣。
- 打完最後一手就**立刻結束 Round**（名次當下確定、檯面清空、順位者自由出牌），
  不必等其他人再 PASS 一輪。
- 座位在**開局時重洗**，所以房長不能用「座位 0」認定 —— 用 `Room.hostID`。

## 測試

日常開發用 `-short`（跳過十萬局隨機對局與窮舉測試），提交前跑完整版：

```bash
GO111MODULE=on go test -short ./...        # 快，日常用
GO111MODULE=on go test ./...               # 完整，約 2~4 分鐘
GO111MODULE=on go test -race -short ./...  # 改 platform 後必跑
```

寫測試時請涵蓋臨界與極端情況，不要只測正常路徑 —— 這個專案的幾個真 bug
都是靠邊界測試與 `-race` 抓出來的。

### 端對端測試的注意事項

`playFullGame` 用四條真的 WebSocket 連線打完一整局。每個客戶端各自收推播，
所以**某一瞬間四個人看到的輪次可能不一致**。挑「現在輪到誰」時必須等到
四個視角一致（`currentActor` 會檢查），否則會挑到一個早就過了回合的玩家而卡死。
同理，`currentActor` 回傳 -1 不代表牌局結束，要另外用 `allFinished` 判斷。

## 洗牌

發牌與排座位用 `internal/shuffle` 的 `Crypto`，它以 `crypto/rand` 為亂數來源。
**不要改回 `math/rand`**：以時間當種子的話，知道伺服器啟動時間就能推算出
後續所有牌局；而且 `*rand.Rand` 併發使用會有資料競爭（`Crypto` 沒有可變狀態）。

洗牌一定要用 Fisher–Yates —— 由後往前，交換對象只從**尚未定案的** `[0, i]`
裡挑。看似等價的「每張牌都跟任意位置交換」會產生明顯偏差（4 張牌時最常見的
排列約是最罕見的 1.9 倍），`TestNaiveShuffleWouldBeBiased` 把這個差異釘住了。
取隨機數用 `randBelow`，它以拒絕取樣避免 `% n` 造成的模數偏差。

測試要重現固定牌局時用 `shuffle.Seeded(seed)`。

## 斷線重連

身分靠 `internal/platform/session.go` 的 token 跨連線保存，不是綁在連線上。
關鍵是分辨兩種「同一個身分出現第二條連線」的情況：`session.dropAt` 非零
代表原本那條已經斷了（重連，接回去），零代表還活著（搶佔，必須擋下來
保護先連上的人）。改動這一段時務必保持這個區分。

斷線時 `markDisconnected` 只在**遊戲進行中**才把房間記進 session ——
還沒開局的話斷線就直接離開房間了，記著會讓重連試圖接回不存在的座位。

有人斷線時 `ActInRoom` 會回 `ErrWaitingForPlayer` 讓牌局暫停，
`Lobby.MarkOffline` 則替他保留座位。斷線是**平台**的概念，遊戲本身不管連線，
所以平台把 `Room.OfflineSeats()` 當參數傳進 `ViewFor`，由各遊戲自己決定
怎麼標進畫面（停用動作按鈕、列出在等誰）。

## 牌局紀錄

每局會在 `-logdir`（預設 `logs/`）底下寫一份紀錄檔，內容足以重現整局。
`match` 層只透過 `Observer` 介面回報事件，**不碰檔案 I/O**；
真正寫檔的是 `internal/games/bigtwo/gamelog`。要加記錄內容請擴充 `Observer`。

紀錄的格式是**各遊戲自己的事**：平台只認得 `platform.Recorder`（只有 `Close`），
由 `Kind.NewRecorder` 開檔、原封不動交還給 `New`，再由遊戲轉回具體型別使用。
