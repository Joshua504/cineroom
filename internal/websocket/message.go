package websocket

import "time"

type Message struct {
	Type       string        `json:"type"`
	RoomID     string        `json:"roomId,omitempty"`
	HostID     string        `json:"hostId,omitempty"`
	Position   float64       `json:"position,omitempty"`
	Playing    *bool         `json:"playing,omitempty"`
	Version    int64         `json:"version,omitempty"`
	UpdatedAt  time.Time     `json:"updatedAt,omitempty"`
	Text       string        `json:"text,omitempty"`
	SenderID   string        `json:"senderId,omitempty"`
	SenderName string        `json:"senderName,omitempty"`
	MemberID   string        `json:"memberId,omitempty"`
	Code       string        `json:"code,omitempty"`
	Users      []Participant `json:"users,omitempty"`
	PingSent   int64         `json:"pingSent,omitempty"`
	Locked     *bool         `json:"locked,omitempty"`
}

type Participant struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	PingMS   int64  `json:"pingMs"`
}
