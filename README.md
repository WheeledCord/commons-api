# Simple Chat App Backend

A robust, minimalist chat application backend built in Go with SQLite. Supports multiple clients via JSON over HTTP and WebSockets.

## Features

- User registration and authentication (bcrypt)
- Create "halls" (servers) with invite codes
- Multiple "rooms" (channels) per hall
- Real-time messaging via WebSockets
- Presence tracking with heartbeat pings
- SQLite database for persistence
- **Default Hall**: All new users automatically join "HKCLB" hall with #general and #summer-of-making channels

## Setup

### Prerequisites

- Go 1.21 or later
- SQLite3

### Installation

1. Clone/download the source files
2. Install dependencies:

```bash
go mod tidy
```

3. Run the server:

```bash
go run .
```

The server will start on `http://localhost:8080` with WebSocket endpoint at `ws://localhost:8080/ws`.

### Database

The SQLite database (`chat.db`) is created automatically in the current directory. To use a custom path:

```bash
go run . /path/to/custom.db
```

## API Endpoints

### Authentication

- `POST /api/register` - Create new user account
- `POST /api/login` - Authenticate user and get session token
- `POST /api/logout` - Invalidate session token

### Halls (Servers)

- `GET /api/halls` - Get user's halls
- `POST /api/halls/create` - Create new hall
- `POST /api/halls/join` - Join hall with invite code

### Rooms (Channels)

- `GET /api/rooms/{hall_id}` - Get rooms in a hall
- `POST /api/rooms/create` - Create new room in hall

### Messages

- `GET /api/messages/{room_id}` - Get recent messages (limit with `?limit=N`)

### WebSocket

- `GET /ws?token={session_token}` - Establish WebSocket connection

## Authentication

All protected endpoints require a Bearer token in the Authorization header:

```
Authorization: Bearer {session_token}
```

WebSocket connections authenticate via query parameter: `?token={session_token}`

## Production Considerations

- Change CORS settings in production
- Add rate limiting
- Use HTTPS/WSS in production
- Consider using a more robust session store
- Add logging configuration
- Monitor database performance

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌─────────────┐
│   Client    │────│  HTTP/WS     │────│  Database   │
│ (Web/CLI)   │    │  Handlers    │    │  (SQLite)   │
└─────────────┘    └──────────────┘    └─────────────┘
                          │
                   ┌──────────────┐
                   │  WebSocket   │
                   │  Manager     │
                   └──────────────┘
```

The backend is completely frontend-agnostic and communicates only via JSON over HTTP and WebSockets.
