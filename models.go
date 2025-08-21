package main

import (
	"time"
)

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	LastSeen     time.Time `json:"last_seen"`
}

type Hall struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	InviteCode string    `json:"invite_code"`
	OwnerID    int       `json:"owner_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type Room struct {
	ID        int       `json:"id"`
	HallID    int       `json:"hall_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	ID        int       `json:"id"`
	RoomID    int       `json:"room_id"`
	UserID    int       `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type HallMember struct {
	ID       int       `json:"id"`
	HallID   int       `json:"hall_id"`
	UserID   int       `json:"user_id"`
	JoinedAt time.Time `json:"joined_at"`
}

// WebSocket message types
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type JoinRoomData struct {
	HallID int `json:"hall_id"`
	RoomID int `json:"room_id"`
}

type SendMessageData struct {
	RoomID  int    `json:"room_id"`
	Content string `json:"content"`
}

type BroadcastMessageData struct {
	Message Message `json:"message"`
	RoomID  int     `json:"room_id"`
}

type PresenceData struct {
	UserID int    `json:"user_id"`
	Status string `json:"status"` // "online" or "offline"
}
