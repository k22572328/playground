// Package shuffle 提供發牌與排座位用的洗牌器。
//
// 牌局的公平性取決於洗牌：玩家不該能從先前的牌局推算出後續的發牌。
// 因此這裡預設使用 crypto/rand，而不是以時間當種子的 math/rand ——
// 後者的種子只有時間這點熵，知道伺服器啟動時間就能重現整個序列。
package shuffle

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand"
)

// Crypto 是以 crypto/rand 為亂數來源的洗牌器。
//
// 它沒有任何可變狀態，所以可以安全地同時給多個房間使用，
// 不需要額外上鎖。零值即可使用。
type Crypto struct{}

// Shuffle 用 Fisher–Yates 把 n 個元素洗成均勻隨機的排列。
func (Crypto) Shuffle(n int, swap func(i, j int)) {
	// 由後往前，每次從 [0, i] 裡挑一個換到位置 i。
	// 這樣每種排列出現的機率完全相同。
	for i := n - 1; i > 0; i-- {
		swap(i, int(randBelow(uint64(i+1))))
	}
}

// randBelow 回傳 [0, n) 之間均勻分布的亂數。
//
// 直接取餘數會讓小的數值稍微容易出現（模數偏差），所以這裡改用
// 拒絕取樣：先算出會造成偏差的尾端範圍，落在那裡就重抽。
func randBelow(n uint64) uint64 {
	if n == 0 {
		panic("shuffle: randBelow 的上界必須大於 0")
	}
	// limit 是最大的、能被 n 整除的邊界；超過它的值會造成偏差。
	limit := ^uint64(0) - (^uint64(0) % n) - 1
	for {
		v := randUint64()
		if v <= limit {
			return v % n
		}
	}
}

// randUint64 從作業系統的亂數來源取 64 個隨機位元。
//
// crypto/rand 在現代系統上不會失敗；真的失敗代表系統的亂數來源壞了，
// 這時繼續發牌只會發出可預測的牌，不如直接中止。
func randUint64() uint64 {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("shuffle: 無法取得系統亂數: %v", err))
	}
	return binary.LittleEndian.Uint64(b[:])
}

// Seeded 回傳一個由固定種子驅動的洗牌器，只給測試與重現牌局使用。
//
// 它的輸出完全可預測，所以不可以用在真正的對局上。
func Seeded(seed int64) *Deterministic {
	return &Deterministic{r: rand.New(rand.NewSource(seed))}
}

// Deterministic 是可重現的洗牌器，供測試使用。
// 它不是併發安全的，一個測試請只在一條 goroutine 裡用它。
type Deterministic struct{ r *rand.Rand }

func (d *Deterministic) Shuffle(n int, swap func(i, j int)) { d.r.Shuffle(n, swap) }
