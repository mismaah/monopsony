// Package ws is the WebSocket gateway: one goroutine pair per connection,
// authenticated by access token, multiplexing any number of rooms.
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"monopsony/server/internal/auth"
	"monopsony/server/internal/game"
	"monopsony/server/internal/lobby"
	"monopsony/server/internal/metrics"
	"monopsony/server/internal/protocol"
	"monopsony/server/internal/ratelimit"
	"monopsony/server/internal/room"
)

// Handler upgrades HTTP connections.
type Handler struct {
	Auth    *auth.Service
	Lobby   *lobby.Manager
	Origins []string // allowed origins (dev: the Vite hosts); empty = same-origin only
	Log     *slog.Logger
	// Metrics receives connection/frame counters; nil = the shared default.
	Metrics *metrics.Metrics
	// FrameRate caps inbound frames per connection; ChatRate caps chat lines.
	// Zero values disable the cap.
	FrameRate ratelimit.Rate
	ChatRate  ratelimit.Rate
}

const (
	outboundBuffer = 256
	writeTimeout   = 10 * time.Second
	readLimit      = 64 * 1024
)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	claims, err := h.Auth.Verify(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.Origins})
	if err != nil {
		return
	}
	c.SetReadLimit(readLimit)
	met := h.Metrics
	if met == nil {
		met = metrics.Default()
	}
	conn := &conn{
		ws: c, userID: claims.Subject, name: claims.Name,
		out: make(chan protocol.Envelope, outboundBuffer), rooms: map[string]room.Handle{},
		lobby: h.Lobby, log: h.Log.With("user", claims.Subject), met: met,
		frames: ratelimit.New(h.FrameRate), chats: ratelimit.New(h.ChatRate),
	}
	met.WSConnections.Inc()
	defer met.WSConnections.Dec()
	conn.run(r.Context())
}

type conn struct {
	ws     *websocket.Conn
	userID string
	name   string
	out    chan protocol.Envelope
	lobby  *lobby.Manager
	log    *slog.Logger
	met    *metrics.Metrics
	frames *ratelimit.Limiter
	chats  *ratelimit.Limiter

	mu     sync.Mutex
	rooms  map[string]room.Handle
	closed bool
}

func (c *conn) UserID() string { return c.userID }

// Send queues a frame; a client that cannot keep up is disconnected rather
// than allowed to stall the room.
func (c *conn) Send(env protocol.Envelope) {
	select {
	case c.out <- env:
	default:
		c.log.Warn("slow websocket client, dropping connection")
		c.ws.Close(websocket.StatusPolicyViolation, "slow consumer")
	}
}

func (c *conn) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer c.cleanup()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case env := <-c.out:
				wctx, wcancel := context.WithTimeout(ctx, writeTimeout)
				err := wsjson.Write(wctx, c.ws, env)
				wcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	c.Send(protocol.MustEncode(protocol.SWelcome, "", protocol.Welcome{UserID: c.userID, Name: c.name}))
	for {
		var env protocol.Envelope
		if err := wsjson.Read(ctx, c.ws, &env); err != nil {
			return
		}
		c.handle(env)
	}
}

func (c *conn) cleanup() {
	c.mu.Lock()
	c.closed = true
	rooms := c.rooms
	c.rooms = map[string]room.Handle{}
	c.mu.Unlock()
	for _, r := range rooms {
		r.Unsubscribe(c)
	}
	c.ws.Close(websocket.StatusNormalClosure, "")
}

func (c *conn) sendError(ref, code, msg string) {
	c.Send(protocol.MustEncode(protocol.SError, ref, protocol.Error{Code: code, Message: msg}))
}

func (c *conn) handle(env protocol.Envelope) {
	c.met.WSMessages.WithLabelValues(env.T).Inc()
	if !c.frames.Allow("") {
		c.met.RateLimited.WithLabelValues("ws_frame").Inc()
		c.sendError(env.Ref, "rate_limited", "too many messages, slow down")
		return
	}
	switch env.T {
	case protocol.CPing:
		c.Send(protocol.Envelope{T: protocol.SPong, Ref: env.Ref})
	case protocol.CJoinGame:
		var p protocol.JoinGame
		if err := json.Unmarshal(env.P, &p); err != nil {
			c.sendError(env.Ref, "bad_request", "malformed JoinGame")
			return
		}
		r, ok := c.lobby.Get(p.GameID)
		if !ok {
			c.sendError(env.Ref, "not_found", "no such game")
			return
		}
		c.mu.Lock()
		c.rooms[p.GameID] = r
		c.mu.Unlock()
		r.Subscribe(c, p.LastSeq)
	case protocol.CLeaveGame:
		var p protocol.LeaveGame
		if err := json.Unmarshal(env.P, &p); err != nil {
			return
		}
		c.mu.Lock()
		r, ok := c.rooms[p.GameID]
		delete(c.rooms, p.GameID)
		c.mu.Unlock()
		if ok {
			r.Unsubscribe(c)
		}
	case protocol.CCommand:
		var p protocol.Command
		if err := json.Unmarshal(env.P, &p); err != nil {
			c.sendError(env.Ref, "bad_request", "malformed Command")
			return
		}
		r, ok := c.lobby.Get(p.GameID)
		if !ok {
			c.sendError(env.Ref, "not_found", "no such game")
			return
		}
		// The acting player is always the connection's user; the client never
		// chooses it.
		payload := p.Payload
		if len(payload) == 0 {
			payload = []byte("{}")
		}
		var withActor map[string]json.RawMessage
		if err := json.Unmarshal(payload, &withActor); err != nil {
			c.sendError(env.Ref, "bad_request", "malformed payload")
			return
		}
		withActor["playerId"], _ = json.Marshal(c.userID)
		merged, _ := json.Marshal(withActor)
		cmd, err := game.DecodeCommand(p.Type, merged)
		if err != nil {
			c.sendError(env.Ref, "bad_request", err.Error())
			return
		}
		if err := r.Command(c.userID, cmd); err != nil {
			code, msg := errorCode(err)
			c.sendError(env.Ref, code, msg)
			return
		}
		c.Send(protocol.MustEncode(protocol.SAck, env.Ref, protocol.Ack{}))
	case protocol.CReady:
		var p struct {
			GameID string `json:"gameId"`
			Ready  bool   `json:"ready"`
		}
		if err := json.Unmarshal(env.P, &p); err != nil {
			return
		}
		if r, ok := c.lobby.Get(p.GameID); ok {
			r.SetReady(c.userID, p.Ready)
		}
	case protocol.CAssetsReady:
		var p protocol.AssetsReady
		if err := json.Unmarshal(env.P, &p); err != nil {
			return
		}
		if r, ok := c.lobby.Get(p.GameID); ok {
			r.AssetsReady(c.userID)
		}
	case protocol.CChat:
		var p protocol.Chat
		if err := json.Unmarshal(env.P, &p); err != nil {
			return
		}
		if !c.chats.Allow("") {
			c.met.RateLimited.WithLabelValues("ws_chat").Inc()
			c.sendError(env.Ref, "rate_limited", "you are chatting too fast")
			return
		}
		if r, ok := c.lobby.Get(p.GameID); ok {
			r.Chat(c.userID, p.Text)
		}
	default:
		c.sendError(env.Ref, "bad_request", "unknown message type "+env.T)
	}
}

// errorCode maps engine/room errors to wire codes.
func errorCode(err error) (string, string) {
	var ge *game.Error
	if errors.As(err, &ge) {
		return ge.Code, ge.Message
	}
	var re *room.Error
	if errors.As(err, &re) {
		return re.Code, re.Message
	}
	return "error", err.Error()
}
