package platform

// roomView 大廳列表或房間內看到的一個房間。
type roomView struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Seats   []slotView `json:"seats"`
	Started bool       `json:"started"`
	Full    bool       `json:"full"`

	// KindID 讓前端知道要載入哪個遊戲的畫面；GameName 供顯示。
	KindID   string `json:"kindId"`
	GameName string `json:"gameName"`
	MaxSeats int    `json:"maxSeats"`

	// CanStart 表示目前人數足以開局。
	CanStart bool `json:"canStart"`

	// YouAreHost 讓前端決定要不要顯示踢人與開始按鈕。
	YouAreHost bool `json:"youAreHost"`
}

// slotView 房間裡的一個位子（尚未開始遊戲時）。
type slotView struct {
	// PlayerID 讓房長的踢人按鈕知道要踢誰。
	PlayerID string `json:"playerId"`
	Name     string `json:"name"`
	IsHost   bool   `json:"isHost"`
	IsYou    bool   `json:"isYou"`
	Offline  bool   `json:"offline"`
}

func newRoomView(r *Room, viewerID string) roomView {
	v := roomView{
		ID:         r.ID,
		Name:       r.Name,
		Started:    r.Started,
		Full:       r.Full(),
		KindID:     r.KindID,
		CanStart:   r.CanStart(),
		YouAreHost: r.IsHost(viewerID),
		Seats:      make([]slotView, 0, len(r.Seats)),
	}
	if k, ok := r.Kind(); ok {
		v.GameName = k.Name
		v.MaxSeats = k.MaxSeats
	}
	for _, s := range r.Seats {
		v.Seats = append(v.Seats, slotView{
			PlayerID: s.PlayerID,
			Name:     s.Name,
			IsHost:   r.IsHost(s.PlayerID),
			IsYou:    s.PlayerID == viewerID,
			Offline:  s.Offline,
		})
	}
	return v
}
