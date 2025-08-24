package main

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// cleanRoomName processes room name according to rules
func cleanRoomName(name string) string {
	// Add # prefix if not present
	if !strings.HasPrefix(name, "#") {
		name = "#" + name
	}
	
	// Convert to lowercase
	name = strings.ToLower(name)
	
	// Replace spaces with dashes
	name = strings.ReplaceAll(name, " ", "-")
	
	// Remove invalid symbols (!@#$%^&*()_=) but keep # at start and allow dashes
	reg := regexp.MustCompile(`[!@$%^&*()_=]+`)
	name = reg.ReplaceAllString(name, "")
	
	// Handle double dashes - replace with single dash
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	
	// Remove trailing dashes
	name = strings.TrimRight(name, "-")
	
	// Limit to 20 characters
	if len(name) > 20 {
		name = name[:20]
		// Remove trailing dash if cut created one
		name = strings.TrimRight(name, "-")
	}
	
	// Must have at least # plus one character
	if len(name) <= 1 {
		return ""
	}
	
	return name
}

type Server struct {
	db        *Database
	auth      *AuthManager
	wsManager *WSManager
}

func NewServer(db *Database) *Server {
	auth := NewAuthManager(db)
	wsManager := NewWSManager(db, auth)

	return &Server{
		db:        db,
		auth:      auth,
		wsManager: wsManager,
	}
}



func (s *Server) RegisterRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Serve static files from ../commons-webui/
	webUIDir := filepath.Join("..", "commons-webui")
	fs := http.FileServer(http.Dir(webUIDir))
	mux.Handle("/", fs)

	// Auth endpoints
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.auth.RequireAuth(s.handleLogout))

	// Hall management
	mux.HandleFunc("/api/halls/create", s.auth.RequireAuth(s.handleCreateHall))
	mux.HandleFunc("/api/halls/join", s.auth.RequireAuth(s.handleJoinHall))
	mux.HandleFunc("/api/halls/leave", s.auth.RequireAuth(s.handleLeaveHall))
	mux.HandleFunc("/api/halls/give-admin", s.auth.RequireAuth(s.handleGiveAdmin))
	mux.HandleFunc("/api/halls", s.auth.RequireAuth(s.handleHalls))
	mux.HandleFunc("/api/halls/", s.auth.RequireAuth(s.handleHallWithID))

	// Room management
	mux.HandleFunc("/api/rooms/create", s.auth.RequireAuth(s.handleCreateRoom))
	mux.HandleFunc("/api/rooms/delete", s.auth.RequireAuth(s.handleDeleteRoom))
	mux.HandleFunc("/api/rooms/", s.auth.RequireAuth(s.handleRoomsWithID))
	mux.HandleFunc("/api/messages/", s.auth.RequireAuth(s.handleMessages))

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.handleWebSocket)

	return mux
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		respondError(w, "Username and password required", http.StatusBadRequest)
		return
	}

	user, err := s.db.CreateUser(req.Username, req.Password)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			respondError(w, "Username already exists", http.StatusConflict)
			return
		}
		respondError(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Add user to default HKCLB hall
	if err := s.db.AddUserToDefaultHall(user.ID); err != nil {
		log.Printf("Warning: Failed to add user %s to default hall: %v", user.Username, err)
		// Don't fail registration if this fails, just log it
	}

	session, err := s.auth.CreateSession(user)
	if err != nil {
		respondError(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"user":  user,
		"token": session.Token,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	user, err := s.db.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		respondError(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	session, err := s.auth.CreateSession(user)
	if err != nil {
		respondError(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"user":  user,
		"token": session.Token,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := s.auth.ExtractToken(r)
	s.auth.DeleteSession(token)

	respondJSON(w, map[string]string{"status": "logged out"})
}

func (s *Server) handleHalls(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	halls, err := s.db.GetUserHalls(session.UserID)
	if err != nil {
		respondError(w, "Failed to fetch halls", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"halls": halls,
	})
}

func (s *Server) handleCreateHall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		respondError(w, "Hall name required", http.StatusBadRequest)
		return
	}

	hall, err := s.db.CreateHall(req.Name, session.UserID)
	if err != nil {
		respondError(w, "Failed to create hall", http.StatusInternalServerError)
		return
	}

	// Create default "#general" room
	_, err = s.db.CreateRoom(hall.ID, "#general")
	if err != nil {
		respondError(w, "Failed to create default room", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"hall": hall,
	})
}

func (s *Server) handleJoinHall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		InviteCode string `json:"invite_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.InviteCode == "" {
		respondError(w, "Invite code required", http.StatusBadRequest)
		return
	}

	err := s.db.JoinHall(session.UserID, req.InviteCode)
	if err != nil {
		respondError(w, "Invalid invite code or already member", http.StatusBadRequest)
		return
	}

	hall, err := s.db.GetHallByInviteCode(req.InviteCode)
	if err != nil {
		respondError(w, "Failed to get hall info", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"hall": hall,
	})
}

func (s *Server) handleLeaveHall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		HallID int `json:"hall_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.HallID == 0 {
		respondError(w, "Hall ID required", http.StatusBadRequest)
		return
	}

	err := s.db.LeaveHall(session.UserID, req.HallID)
	if err != nil {
		respondError(w, "Failed to leave hall", http.StatusBadRequest)
		return
	}

	respondJSON(w, map[string]interface{}{
		"success": true,
	})
}

func (s *Server) handleRoomsWithID(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract path /api/rooms/{id} or /api/rooms/{id}/action
	path := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
	parts := strings.Split(path, "/")
	
	if len(parts) == 1 {
		// Handle /api/rooms/{hall_id} - get rooms in hall
		s.handleRooms(w, r)
		return
	}
	
	if len(parts) == 2 && parts[1] == "delete" {
		// Handle /api/rooms/{room_id}/delete
		s.handleDeleteRoomByID(w, r, parts[0])
		return
	}
	
	respondError(w, "Invalid URL format", http.StatusNotFound)
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract hall ID from URL path /api/rooms/{hall_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
	hallID, err := strconv.Atoi(path)
	if err != nil {
		respondError(w, "Invalid hall ID", http.StatusBadRequest)
		return
	}

	// Check if user is member of hall
	isMember, err := s.db.IsUserInHall(session.UserID, hallID)
	if err != nil || !isMember {
		respondError(w, "Access denied", http.StatusForbidden)
		return
	}

	rooms, err := s.db.GetHallRooms(hallID)
	if err != nil {
		respondError(w, "Failed to fetch rooms", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"rooms": rooms,
	})
}

func (s *Server) handleDeleteRoomByID(w http.ResponseWriter, r *http.Request, roomIDStr string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	roomID, err := strconv.Atoi(roomIDStr)
	if err != nil {
		respondError(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	// Get room to check ownership
	room, err := s.db.GetRoomByID(roomID)
	if err != nil {
		respondError(w, "Room not found", http.StatusNotFound)
		return
	}

	// Get hall to check if user is owner
	hall, err := s.db.GetHallByID(room.HallID)
	if err != nil {
		respondError(w, "Hall not found", http.StatusNotFound)
		return
	}

	if hall.OwnerID != session.UserID {
		respondError(w, "Only hall owner can delete rooms", http.StatusForbidden)
		return
	}

	err = s.db.DeleteRoom(roomID)
	if err != nil {
		respondError(w, "Failed to delete room", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"status": "room deleted"})
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		HallID int    `json:"hall_id"`
		Name   string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.HallID == 0 {
		respondError(w, "Hall ID and room name required", http.StatusBadRequest)
		return
	}

	// Clean and validate room name
	cleanName := cleanRoomName(req.Name)
	if cleanName == "" {
		respondError(w, "Invalid room name", http.StatusBadRequest)
		return
	}

	// Check if user is member of hall
	isMember, err := s.db.IsUserInHall(session.UserID, req.HallID)
	if err != nil || !isMember {
		respondError(w, "Access denied", http.StatusForbidden)
		return
	}

	room, err := s.db.CreateRoom(req.HallID, cleanName)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			respondError(w, "Room name already exists in this hall", http.StatusConflict)
			return
		}
		respondError(w, "Failed to create room", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"room": room,
	})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract room ID from URL path /api/messages/{room_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/messages/")
	roomID, err := strconv.Atoi(path)
	if err != nil {
		respondError(w, "Invalid room ID", http.StatusBadRequest)
		return
	}

	// Get room to check hall membership
	room, err := s.db.GetRoomByID(roomID)
	if err != nil {
		respondError(w, "Room not found", http.StatusNotFound)
		return
	}

	// Check if user is member of hall
	isMember, err := s.db.IsUserInHall(session.UserID, room.HallID)
	if err != nil || !isMember {
		respondError(w, "Access denied", http.StatusForbidden)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	messages, err := s.db.GetRoomMessages(roomID, limit, offset)
	if err != nil {
		respondError(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"messages": messages,
	})
}

func (s *Server) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		RoomID   int    `json:"room_id,omitempty"`
		RoomName string `json:"room_name,omitempty"`
		HallID   int    `json:"hall_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var room *Room
	var err error
	
	// Get room by ID or by name+hall
	if req.RoomID != 0 {
		room, err = s.db.GetRoomByID(req.RoomID)
	} else if req.RoomName != "" && req.HallID != 0 {
		room, err = s.db.GetRoomByName(req.HallID, req.RoomName)
	} else {
		respondError(w, "Either room_id or (room_name + hall_id) required", http.StatusBadRequest)
		return
	}
	
	if err != nil {
		respondError(w, "Room not found", http.StatusNotFound)
		return
	}

	// Get hall to check if user is owner
	hall, err := s.db.GetHallByID(room.HallID)
	if err != nil {
		respondError(w, "Hall not found", http.StatusNotFound)
		return
	}

	if hall.OwnerID != session.UserID {
		respondError(w, "Only hall owner can delete rooms", http.StatusForbidden)
		return
	}

	err = s.db.DeleteRoom(req.RoomID)
	if err != nil {
		respondError(w, "Failed to delete room", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"status": "room deleted"})
}

func (s *Server) handleHallWithID(w http.ResponseWriter, r *http.Request) {
	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract hall ID from URL path /api/halls/{hall_id}/action
	path := strings.TrimPrefix(r.URL.Path, "/api/halls/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		respondError(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	hallID, err := strconv.Atoi(parts[0])
	if err != nil {
		respondError(w, "Invalid hall ID", http.StatusBadRequest)
		return
	}

	action := parts[1]

	// Check if user owns the hall
	hall, err := s.db.GetHallByID(hallID)
	if err != nil {
		respondError(w, "Hall not found", http.StatusNotFound)
		return
	}

	if hall.OwnerID != session.UserID {
		respondError(w, "Only hall owner can perform admin actions", http.StatusForbidden)
		return
	}

	switch action {
	case "regenerate-invite":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		newCode, err := s.db.RegenerateInviteCode(hallID)
		if err != nil {
			respondError(w, "Failed to regenerate invite code", http.StatusInternalServerError)
			return
		}
		respondJSON(w, map[string]interface{}{
			"invite_code": newCode,
		})
	case "delete":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Prevent deletion of default HKCLB hall
		if hall.Name == "HKCLB" {
			respondError(w, "Cannot delete default hall", http.StatusForbidden)
			return
		}
		err := s.db.DeleteHall(hallID)
		if err != nil {
			respondError(w, "Failed to delete hall", http.StatusInternalServerError)
			return
		}
		respondJSON(w, map[string]string{"status": "hall deleted"})
	default:
		respondError(w, "Unknown action", http.StatusNotFound)
	}
}

func (s *Server) handleGiveAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Username string `json:"username"`
		HallID   int    `json:"hall_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Check if user owns the hall
	hall, err := s.db.GetHallByID(req.HallID)
	if err != nil {
		respondError(w, "Hall not found", http.StatusNotFound)
		return
	}

	if hall.OwnerID != session.UserID {
		respondError(w, "Only hall owner can give admin rights", http.StatusForbidden)
		return
	}

	// Get target user
	targetUser, err := s.db.GetUserByUsername(req.Username)
	if err != nil {
		respondError(w, "User not found", http.StatusNotFound)
		return
	}

	// Check if user is member of hall
	isMember, err := s.db.IsUserInHall(targetUser.ID, req.HallID)
	if err != nil || !isMember {
		respondError(w, "User is not a member of this hall", http.StatusBadRequest)
		return
	}

	// For now, just respond with success (admin system would need additional tables)
	respondJSON(w, map[string]string{
		"status": "Admin rights feature not fully implemented yet",
		"message": "This would grant admin rights in a full implementation",
	})
}

func (s *Server) handleDeleteHall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	if session == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		HallID int `json:"hall_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Check if user owns the hall
	hall, err := s.db.GetHallByID(req.HallID)
	if err != nil {
		respondError(w, "Hall not found", http.StatusNotFound)
		return
	}

	if hall.OwnerID != session.UserID {
		respondError(w, "Only hall owner can delete hall", http.StatusForbidden)
		return
	}

	// Prevent deletion of default HKCLB hall
	if hall.Name == "HKCLB" {
		respondError(w, "Cannot delete default hall", http.StatusForbidden)
		return
	}

	err = s.db.DeleteHall(req.HallID)
	if err != nil {
		respondError(w, "Failed to delete hall", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"status": "hall deleted"})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract token from query parameter for WebSocket auth
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	session, err := s.auth.ValidateSession(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	s.wsManager.HandleConnection(w, r, session)
}
