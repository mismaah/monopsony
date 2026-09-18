// Package protocol defines every message exchanged over the WebSocket. It is
// the single source of truth for the TypeScript client types (generated with
// tygo into packages/protocol).
package protocol

import (
	"encoding/json"

	"monopsony/server/internal/game"
)

// Envelope wraps every frame in both directions.
type Envelope struct {
	T   string          `json:"t"`
	Ref string          `json:"ref,omitempty"` // client-chosen id echoed on Ack/Error
	P   json.RawMessage `json:"p,omitempty"`
}

// ---- client -> server ----------------------------------------------------------

// Client message types.
const (
	CJoinGame  = "JoinGame"  // subscribe to a room (lobby or in-progress)
	CLeaveGame = "LeaveGame" // unsubscribe
	CCommand   = "Command"   // a game.Command
	CReady     = "Ready"     // lobby: toggle ready
	CChat      = "Chat"
	CPing      = "Ping"
)

type JoinGame struct {
	GameID  string `json:"gameId"`
	LastSeq int    `json:"lastSeq"` // events after this seq are replayed
}

type LeaveGame struct {
	GameID string `json:"gameId"`
}

type Command struct {
	GameID  string          `json:"gameId"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Chat struct {
	GameID string `json:"gameId"`
	Text   string `json:"text"`
}

// ---- server -> client ----------------------------------------------------------

// Server message types.
const (
	SWelcome  = "Welcome"
	SSnapshot = "Snapshot"
	SEvent    = "Event"
	SUpdate   = "Update"
	SLobby    = "Lobby"
	SChat     = "Chat"
	SKicked   = "Kicked" // to the removed user only: they no longer hold a seat
	SAck      = "Ack"
	SError    = "Error"
	SPong     = "Pong"
)

type Welcome struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
}

// SeatInfo is what everyone can see about a seat.
type SeatInfo struct {
	PlayerID  string            `json:"playerId"`
	Name      string            `json:"name"`
	IsBot     bool              `json:"isBot"`
	Connected bool              `json:"connected"`
	Ready     bool              `json:"ready"`
	Host      bool              `json:"host"`
	Loadout   map[string]string `json:"loadout,omitempty"` // cosmetic slot -> item id
}

// Lobby is the pre-game room view.
type Lobby struct {
	GameID      string     `json:"gameId"`
	Status      string     `json:"status"` // "lobby" | "in_progress" | "finished"
	Name        string     `json:"name"`
	Visibility  string     `json:"visibility"` // "public" | "private"
	InviteCode  string     `json:"inviteCode,omitempty"`
	MaxPlayers  int        `json:"maxPlayers"`
	ConfigID    string     `json:"configId"`
	Rules       game.Rules `json:"rules"`
	Seats       []SeatInfo `json:"seats"`
	TurnSeconds int        `json:"turnSeconds"`
	// Node hosts the game (cluster deployments; empty on a single server).
	Node string `json:"node,omitempty"`
}

// Snapshot is the full game view sent on join/reconnect.
type Snapshot struct {
	GameID    string        `json:"gameId"`
	Config    *game.Config  `json:"config"`
	State     *game.State   `json:"state"`
	Seats     []SeatInfo    `json:"seats"`
	Legal     []game.Action `json:"legal"`
	WaitingOn []string      `json:"waitingOn"`
	Deadline  int64         `json:"deadline,omitempty"` // unix ms when the current wait times out
}

// Event is one game event with its sequence number.
type Event struct {
	GameID  string          `json:"gameId"`
	Seq     int             `json:"seq"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Update is sent to each subscriber after every state change: the new
// authoritative state plus that player's legal actions. Events describe *how*
// the state changed (for animation); Update says *what it is now*.
type Update struct {
	GameID    string        `json:"gameId"`
	Seq       int           `json:"seq"`
	State     *game.State   `json:"state"`
	Seats     []SeatInfo    `json:"seats"`
	Actions   []game.Action `json:"actions"`
	WaitingOn []string      `json:"waitingOn"`
	Deadline  int64         `json:"deadline,omitempty"`
}

type ChatMessage struct {
	GameID   string `json:"gameId"`
	PlayerID string `json:"playerId"`
	Name     string `json:"name"`
	Text     string `json:"text"`
	At       int64  `json:"at"`
}

// Kicked tells a user the host removed them from a game. Mid-game their
// seat carries on under a bot.
type Kicked struct {
	GameID   string `json:"gameId"`
	PlayerID string `json:"playerId"`
}

type Ack struct {
	Seq int `json:"seq"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Encode builds an envelope from a typed payload.
func Encode(t, ref string, payload any) (Envelope, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Envelope{}, err
		}
		raw = b
	}
	return Envelope{T: t, Ref: ref, P: raw}, nil
}

// MustEncode is Encode for payloads that cannot fail to marshal.
func MustEncode(t, ref string, payload any) Envelope {
	env, err := Encode(t, ref, payload)
	if err != nil {
		panic(err)
	}
	return env
}
