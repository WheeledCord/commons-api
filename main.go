package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	// Initialize database
	dbPath := "chat.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	db, err := NewDatabase(dbPath)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	if err := db.CreateTables(); err != nil {
		log.Fatal("Failed to create tables:", err)
	}

	if err := db.EnsureDefaultHall(); err != nil {
		log.Fatal("Failed to ensure default hall:", err)
	}

	// Initialize server
	server := NewServer(db)

	// Setup routes
	mux := server.RegisterRoutes()

	// Add CORS middleware
	handler := corsMiddleware(mux)

	log.Println("Chat server starting on :8080")
	log.Println("WebSocket endpoint: ws://localhost:8080/ws")
	log.Println("API endpoints: http://localhost:8080/api/*")

	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal("Server failed:", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
