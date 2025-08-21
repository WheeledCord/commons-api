package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WSManager struct {
	db          *Database
	auth        *AuthManager
	clients     map[*WSClient]bool
	rooms       map[int][]*WSClient
	broadcast   chan BroadcastMsg
	register    chan *WSClient
	unregister  chan *WSClient
	mutex       sync.RWMutex
}

type WSClient struct {
	conn       *websocket.Conn
	session    *Session
	send       chan []byte
	manager    *WSManager
	rooms      map[int]bool
	lastPing   time.Time
}

type BroadcastMsg struct {
	RoomID  int
	Message []byte
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

func NewWSManager(db *Database, auth *AuthManager) *WSManager {
	manager := &WSManager{
		db:         db,
		auth:       auth,
		clients:    make(map[*WSClient]bool),
		rooms:      make(map[int][]*WSClient),
		broadcast:  make(chan BroadcastMsg),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
	
	go manager.run()
	return manager
}

func (m *WSManager) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-m.register:
			m.mutex.Lock()
			m.clients[client] = true
			m.mutex.Unlock()
			log.Printf("Client connected: %s", client.session.Username)

		case client := <-m.unregister:
			m.mutex.Lock()
			if _, ok := m.clients[client]; ok {
				delete(m.clients, client)
				close(client.send)
				
				// Remove client from all rooms
				for roomID := range client.rooms {
					m.removeClientFromRoom(client, roomID)
				}
			}
			m.mutex.Unlock()
			log.Printf("Client disconnected: %s", client.session.Username)

		case msg := <-m.broadcast:
			m.mutex.RLock()
			clients := m.rooms[msg.RoomID]
			m.mutex.RUnlock()
			
			for _, client := range clients {
				select {
				case client.send <- msg.Message:
				default:
					close(client.send)
					delete(m.clients, client)
				}
			}

		case <-ticker.C:
			m.checkClientHealth()
		}
	}
}

func (m *WSManager) checkClientHealth() {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	for client := range m.clients {
		if time.Since(client.lastPing) > 60*time.Second {
			client.conn.Close()
		}
	}
}

func (m *WSManager) addClientToRoom(client *WSClient, roomID int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if m.rooms[roomID] == nil {
		m.rooms[roomID] = make([]*WSClient, 0)
	}
	
	// Check if client already in room
	for _, c := range m.rooms[roomID] {
		if c == client {
			return
		}
	}
	
	m.rooms[roomID] = append(m.rooms[roomID], client)
	client.rooms[roomID] = true
}

func (m *WSManager) removeClientFromRoom(client *WSClient, roomID int) {
	if m.rooms[roomID] == nil {
		return
	}
	
	for i, c := range m.rooms[roomID] {
		if c == client {
			m.rooms[roomID] = append(m.rooms[roomID][:i], m.rooms[roomID][i+1:]...)
			break
		}
	}
	
	delete(client.rooms, roomID)
}

func (m *WSManager) BroadcastToRoom(roomID int, msgType string, data interface{}) {
	message := WSMessage{
		Type: msgType,
		Data: data,
	}
	
	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal broadcast message: %v", err)
		return
	}
	
	m.broadcast <- BroadcastMsg{
		RoomID:  roomID,
		Message: jsonData,
	}
}

func (m *WSManager) HandleConnection(w http.ResponseWriter, r *http.Request, session *Session) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	client := &WSClient{
		conn:     conn,
		session:  session,
		send:     make(chan []byte, 256),
		manager:  m,
		rooms:    make(map[int]bool),
		lastPing: time.Now(),
	}

	m.register <- client

	// Start goroutines for handling the client
	go client.writePump()
	go client.readPump()
}

func (c *WSClient) readPump() {
	defer func() {
		c.manager.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.lastPing = time.Now()
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("Invalid JSON from client: %v", err)
			continue
		}

		c.handleMessage(msg)
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.conn.WriteMessage(websocket.TextMessage, message)

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) handleMessage(msg WSMessage) {
	switch msg.Type {
	case "join_room":
		c.handleJoinRoom(msg.Data)
	case "leave_room":
		c.handleLeaveRoom(msg.Data)
	case "send_message":
		c.handleSendMessage(msg.Data)
	case "ping":
		c.lastPing = time.Now()
		c.manager.db.UpdateUserLastSeen(c.session.UserID)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

func (c *WSClient) handleJoinRoom(data interface{}) {
	jsonData, _ := json.Marshal(data)
	var joinData JoinRoomData
	if err := json.Unmarshal(jsonData, &joinData); err != nil {
		log.Printf("Invalid join_room data: %v", err)
		return
	}

	// Verify user is member of hall
	isMember, err := c.manager.db.IsUserInHall(c.session.UserID, joinData.HallID)
	if err != nil || !isMember {
		log.Printf("User %s denied access to hall %d", c.session.Username, joinData.HallID)
		return
	}

	// Verify room exists in hall
	room, err := c.manager.db.GetRoomByID(joinData.RoomID)
	if err != nil || room.HallID != joinData.HallID {
		log.Printf("Room %d not found in hall %d", joinData.RoomID, joinData.HallID)
		return
	}

	c.manager.addClientToRoom(c, joinData.RoomID)
	log.Printf("User %s joined room %d", c.session.Username, joinData.RoomID)
}

func (c *WSClient) handleLeaveRoom(data interface{}) {
	jsonData, _ := json.Marshal(data)
	var roomData struct {
		RoomID int `json:"room_id"`
	}
	if err := json.Unmarshal(jsonData, &roomData); err != nil {
		log.Printf("Invalid leave_room data: %v", err)
		return
	}

	c.manager.mutex.Lock()
	c.manager.removeClientFromRoom(c, roomData.RoomID)
	c.manager.mutex.Unlock()
	
	log.Printf("User %s left room %d", c.session.Username, roomData.RoomID)
}

func (c *WSClient) handleSendMessage(data interface{}) {
	jsonData, _ := json.Marshal(data)
	var sendData SendMessageData
	if err := json.Unmarshal(jsonData, &sendData); err != nil {
		log.Printf("Invalid send_message data: %v", err)
		return
	}

	if sendData.Content == "" {
		return
	}

	//verify user is in the room
	if !c.rooms[sendData.RoomID] {
		log.Printf("User %s not in room %d", c.session.Username, sendData.RoomID)
		return
	}

	//save message to database
	message, err := c.manager.db.SaveMessage(sendData.RoomID, c.session.UserID, sendData.Content)
	if err != nil {
		log.Printf("Failed to save message: %v", err)
		return
	}

	//nroadcast to all clients in room
	c.manager.BroadcastToRoom(sendData.RoomID, "new_message", BroadcastMessageData{
		Message: *message,
		RoomID:  sendData.RoomID,
	})
}
