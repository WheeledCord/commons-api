# commons-api

A minimalistic chat application backend built in Go with SQLite. Supports multiple clients via JSON over HTTP and WebSockets.

## features

- user registration and authentication (bcrypt)
- create halls with invite codes
- Multiple rooms per hall
- realtime messaging via ws
- presence tracking with heartbeat pings
- SQLite database for persistence

## setup

### stuff you need

- Go 1.21 or later
- SQLite3

### installation

1. download the source files
2. install dependencies:

```bash
go mod tidy
```

3. run the server:

```bash
go run .
```

the server will start on `http://localhost:8080` with ws endpoint at `ws://localhost:8080/ws`.

### database

the SQLite database (`chat.db`) is created automatically in the current directory. to use a custom path:

```bash
go run . /path/to/custom.db
```

## endpoints

### auth

- `POST /api/register` creates new user account
- `POST /api/login` authenticates user and gets session token
- `POST /api/logout` invalidates session token

### halls

- `GET /api/halls` get user's halls
- `POST /api/halls/create` create new hall
- `POST /api/halls/join` join hall with invite code

### Rooms

- `GET /api/rooms/{hall_id}` - get rooms in a hall
- `POST /api/rooms/create` - create new room in hall

### Messages

- `GET /api/messages/{room_id}` - get recent messages (limit with `?limit=N`)

### Ws

- `GET /ws?token={session_token}` - establish ws connection

## Auth

all protected endpoints require a bearer token in the auth header:

```
Authorization: Bearer {session_token}
```

websocket connections authenticate via query parameter: `?token={session_token}`
