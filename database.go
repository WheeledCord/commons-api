package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type Database struct {
	db *sql.DB
}

func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	
	if err := db.Ping(); err != nil {
		return nil, err
	}
	
	return &Database{db: db}, nil
}

func (d *Database) CreateTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username VARCHAR(50) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS halls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(100) NOT NULL,
		invite_code VARCHAR(20) UNIQUE NOT NULL,
		owner_id INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (owner_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS rooms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hall_id INTEGER NOT NULL,
		name VARCHAR(100) NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (hall_id) REFERENCES halls(id) ON DELETE CASCADE,
		UNIQUE(hall_id, name)
	);

	CREATE TABLE IF NOT EXISTS hall_members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hall_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (hall_id) REFERENCES halls(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
		UNIQUE(hall_id, user_id)
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		room_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_room_created ON messages(room_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_hall_members_hall ON hall_members(hall_id);
	CREATE INDEX IF NOT EXISTS idx_hall_members_user ON hall_members(user_id);
	CREATE INDEX IF NOT EXISTS idx_rooms_hall ON rooms(hall_id);
	`
	
	_, err := d.db.Exec(schema)
	return err
}

func (d *Database) CreateUser(username, password string) (*User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	result, err := d.db.Exec(
		"INSERT INTO users (username, password_hash) VALUES (?, ?)",
		username, string(hashedPassword),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return d.GetUserByID(int(id))
}

func (d *Database) AuthenticateUser(username, password string) (*User, error) {
	user, err := d.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, err
	}

	d.UpdateUserLastSeen(user.ID)
	return user, nil
}

func (d *Database) GetUserByID(userID int) (*User, error) {
	user := &User{}
	err := d.db.QueryRow(
		"SELECT id, username, password_hash, created_at, last_seen FROM users WHERE id = ?",
		userID,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt, &user.LastSeen)
	
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (d *Database) GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := d.db.QueryRow(
		"SELECT id, username, password_hash, created_at, last_seen FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt, &user.LastSeen)
	
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (d *Database) UpdateUserLastSeen(userID int) error {
	_, err := d.db.Exec(
		"UPDATE users SET last_seen = CURRENT_TIMESTAMP WHERE id = ?",
		userID,
	)
	return err
}

func generateInviteCode() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (d *Database) CreateHall(name string, ownerID int) (*Hall, error) {
	inviteCode, err := generateInviteCode()
	if err != nil {
		return nil, err
	}

	result, err := d.db.Exec(
		"INSERT INTO halls (name, invite_code, owner_id) VALUES (?, ?, ?)",
		name, inviteCode, ownerID,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Add owner as member
	_, err = d.db.Exec(
		"INSERT INTO hall_members (hall_id, user_id) VALUES (?, ?)",
		id, ownerID,
	)
	if err != nil {
		return nil, err
	}

	return d.GetHallByID(int(id))
}

func (d *Database) GetHallByID(hallID int) (*Hall, error) {
	hall := &Hall{}
	err := d.db.QueryRow(
		"SELECT id, name, invite_code, owner_id, created_at FROM halls WHERE id = ?",
		hallID,
	).Scan(&hall.ID, &hall.Name, &hall.InviteCode, &hall.OwnerID, &hall.CreatedAt)
	
	if err != nil {
		return nil, err
	}
	return hall, nil
}

func (d *Database) GetHallByInviteCode(inviteCode string) (*Hall, error) {
	hall := &Hall{}
	err := d.db.QueryRow(
		"SELECT id, name, invite_code, owner_id, created_at FROM halls WHERE invite_code = ?",
		inviteCode,
	).Scan(&hall.ID, &hall.Name, &hall.InviteCode, &hall.OwnerID, &hall.CreatedAt)
	
	if err != nil {
		return nil, err
	}
	return hall, nil
}

func (d *Database) JoinHall(userID int, inviteCode string) error {
	hall, err := d.GetHallByInviteCode(inviteCode)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(
		"INSERT OR IGNORE INTO hall_members (hall_id, user_id) VALUES (?, ?)",
		hall.ID, userID,
	)
	return err
}

func (d *Database) GetUserHalls(userID int) ([]Hall, error) {
	rows, err := d.db.Query(`
		SELECT h.id, h.name, h.invite_code, h.owner_id, h.created_at 
		FROM halls h 
		JOIN hall_members hm ON h.id = hm.hall_id 
		WHERE hm.user_id = ?
		ORDER BY h.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	halls := make([]Hall, 0) // Initialize as empty slice, not nil
	for rows.Next() {
		var hall Hall
		err := rows.Scan(&hall.ID, &hall.Name, &hall.InviteCode, &hall.OwnerID, &hall.CreatedAt)
		if err != nil {
			return nil, err
		}
		halls = append(halls, hall)
	}
	return halls, nil
}

func (d *Database) CreateRoom(hallID int, name string) (*Room, error) {
	result, err := d.db.Exec(
		"INSERT INTO rooms (hall_id, name) VALUES (?, ?)",
		hallID, name,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return d.GetRoomByID(int(id))
}

func (d *Database) GetRoomByID(roomID int) (*Room, error) {
	room := &Room{}
	err := d.db.QueryRow(
		"SELECT id, hall_id, name, created_at FROM rooms WHERE id = ?",
		roomID,
	).Scan(&room.ID, &room.HallID, &room.Name, &room.CreatedAt)
	
	if err != nil {
		return nil, err
	}
	return room, nil
}

func (d *Database) GetHallRooms(hallID int) ([]Room, error) {
	rows, err := d.db.Query(
		"SELECT id, hall_id, name, created_at FROM rooms WHERE hall_id = ? ORDER BY created_at ASC",
		hallID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rooms := make([]Room, 0) // Initialize as empty slice, not nil
	for rows.Next() {
		var room Room
		err := rows.Scan(&room.ID, &room.HallID, &room.Name, &room.CreatedAt)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	return rooms, nil
}

func (d *Database) IsUserInHall(userID, hallID int) (bool, error) {
	var count int
	err := d.db.QueryRow(
		"SELECT COUNT(*) FROM hall_members WHERE user_id = ? AND hall_id = ?",
		userID, hallID,
	).Scan(&count)
	return count > 0, err
}

func (d *Database) SaveMessage(roomID, userID int, content string) (*Message, error) {
	result, err := d.db.Exec(
		"INSERT INTO messages (room_id, user_id, content) VALUES (?, ?, ?)",
		roomID, userID, content,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return d.GetMessageByID(int(id))
}

func (d *Database) GetMessageByID(messageID int) (*Message, error) {
	message := &Message{}
	err := d.db.QueryRow(`
		SELECT m.id, m.room_id, m.user_id, u.username, m.content, m.created_at 
		FROM messages m 
		JOIN users u ON m.user_id = u.id 
		WHERE m.id = ?
	`, messageID).Scan(&message.ID, &message.RoomID, &message.UserID, &message.Username, &message.Content, &message.CreatedAt)
	
	if err != nil {
		return nil, err
	}
	return message, nil
}

func (d *Database) GetRoomMessages(roomID int, limit int) ([]Message, error) {
	rows, err := d.db.Query(`
		SELECT m.id, m.room_id, m.user_id, u.username, m.content, m.created_at 
		FROM messages m 
		JOIN users u ON m.user_id = u.id 
		WHERE m.room_id = ? 
		ORDER BY m.created_at DESC 
		LIMIT ?
	`, roomID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]Message, 0) // Initialize as empty slice, not nil
	for rows.Next() {
		var message Message
		err := rows.Scan(&message.ID, &message.RoomID, &message.UserID, &message.Username, &message.Content, &message.CreatedAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func (d *Database) EnsureDefaultHall() error {
	// Check if HKCLB hall already exists
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM halls WHERE name = ?", "HKCLB").Scan(&count)
	if err != nil {
		return err
	}
	
	if count > 0 {
		return nil // Already exists
	}
	
	// Create system user if it doesn't exist
	_, err = d.db.Exec(`
		INSERT OR IGNORE INTO users (username, password_hash) 
		VALUES ('system', '$2a$10$dummy.hash.for.system.user')
	`)
	if err != nil {
		return err
	}
	
	// Get system user ID
	var systemUserID int
	err = d.db.QueryRow("SELECT id FROM users WHERE username = ?", "system").Scan(&systemUserID)
	if err != nil {
		return err
	}
	
	// Create the HKCLB hall
	hall, err := d.CreateHall("HKCLB", systemUserID)
	if err != nil {
		return err
	}
	
	// Create the required rooms
	_, err = d.CreateRoom(hall.ID, "#general")
	if err != nil {
		return err
	}
	
	_, err = d.CreateRoom(hall.ID, "#summer-of-making")
	if err != nil {
		return err
	}
	
	return nil
}

func (d *Database) AddUserToDefaultHall(userID int) error {
	// Get the HKCLB hall
	var hallID int
	err := d.db.QueryRow("SELECT id FROM halls WHERE name = ?", "HKCLB").Scan(&hallID)
	if err != nil {
		return err
	}
	
	// Add user to the hall
	_, err = d.db.Exec(
		"INSERT OR IGNORE INTO hall_members (hall_id, user_id) VALUES (?, ?)",
		hallID, userID,
	)
	return err
}

func (d *Database) DeleteRoom(roomID int) error {
	_, err := d.db.Exec("DELETE FROM rooms WHERE id = ?", roomID)
	return err
}

func (d *Database) RegenerateInviteCode(hallID int) (string, error) {
	newCode, err := generateInviteCode()
	if err != nil {
		return "", err
	}
	
	_, err = d.db.Exec("UPDATE halls SET invite_code = ? WHERE id = ?", newCode, hallID)
	if err != nil {
		return "", err
	}
	
	return newCode, nil
}

func (d *Database) DeleteHall(hallID int) error {
	_, err := d.db.Exec("DELETE FROM halls WHERE id = ?", hallID)
	return err
}

func (d *Database) GetRoomByName(hallID int, roomName string) (*Room, error) {
	room := &Room{}
	err := d.db.QueryRow(
		"SELECT id, hall_id, name, created_at FROM rooms WHERE hall_id = ? AND name = ?",
		hallID, roomName,
	).Scan(&room.ID, &room.HallID, &room.Name, &room.CreatedAt)
	
	if err != nil {
		return nil, err
	}
	return room, nil
}

func (d *Database) Close() error {
	return d.db.Close()
}
