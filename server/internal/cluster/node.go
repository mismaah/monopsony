package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"monopsony/server/internal/game"
	"monopsony/server/internal/ids"
	"monopsony/server/internal/metrics"
	"monopsony/server/internal/protocol"
	"monopsony/server/internal/room"
	"monopsony/server/internal/store"
)

const (
	keyPrefix  = "mp:"
	aliveTTL   = 15 * time.Second
	claimTTL   = 5 * time.Second
	rpcTimeout = 5 * time.Second
)

// Options tune a node.
type Options struct {
	// ID identifies this process; random when empty.
	ID     string
	Logger *slog.Logger
	// Metrics receives RPC counters; nil = the shared default.
	Metrics *metrics.Metrics
	// Heartbeat overrides the liveness refresh interval (tests).
	Heartbeat time.Duration
}

// Node is this process's membership in the cluster.
type Node struct {
	ID  string
	bus Bus
	log *slog.Logger
	met *metrics.Metrics

	local    func(gameID string) (room.Handle, bool)
	localIDs func() []string

	out    chan outbound
	stop   chan struct{}
	once   sync.Once
	cancel func()
	wg     sync.WaitGroup

	mu       sync.Mutex
	pending  map[string]chan message
	proxies  map[string]*RemoteRoom // gameID -> proxy (caller side)
	conns    map[string]*proxyConn  // connID -> local subscriber (caller side)
	remote   map[string]*remoteSub  // peer|conn -> subscriber (owner side)
	rejoined int
}

type outbound struct {
	channel string
	payload []byte
}

// message is everything that travels on a node's channel.
type message struct {
	Kind string `json:"k"` // rpc | reply | frame

	ID     string          `json:"id,omitempty"`
	From   string          `json:"from,omitempty"`
	Game   string          `json:"g,omitempty"`
	Method string          `json:"m,omitempty"`
	Args   json.RawMessage `json:"a,omitempty"`

	Result json.RawMessage `json:"r,omitempty"`
	Error  *rpcError       `json:"e,omitempty"`

	Conn string             `json:"c,omitempty"`
	Env  *protocol.Envelope `json:"env,omitempty"`
}

type rpcError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrNoOwner is returned when a game has no live owner.
var ErrNoOwner = errors.New("cluster: game has no live owner")

// Join starts a node on bus: it subscribes to its own channel and begins
// heartbeating. Attach must be called before games are served.
func Join(ctx context.Context, bus Bus, opts Options) (*Node, error) {
	if opts.ID == "" {
		opts.ID = ids.New()[:12]
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Metrics == nil {
		opts.Metrics = metrics.Default()
	}
	if opts.Heartbeat <= 0 {
		opts.Heartbeat = aliveTTL / 3
	}
	n := &Node{
		ID: opts.ID, bus: bus, log: opts.Logger.With("node", opts.ID), met: opts.Metrics,
		local:    func(string) (room.Handle, bool) { return nil, false },
		localIDs: func() []string { return nil },
		out:      make(chan outbound, 8192), stop: make(chan struct{}),
		pending: map[string]chan message{}, proxies: map[string]*RemoteRoom{},
		conns: map[string]*proxyConn{}, remote: map[string]*remoteSub{},
	}
	if err := bus.Set(ctx, n.aliveKey(n.ID), "1", aliveTTL); err != nil {
		return nil, err
	}
	cancel, err := bus.Subscribe(ctx, n.channel(n.ID), n.handle)
	if err != nil {
		return nil, err
	}
	n.cancel = cancel
	n.wg.Add(2)
	go n.writer()
	go n.heartbeat(opts.Heartbeat)
	n.log.Info("cluster: joined")
	return n, nil
}

// Attach tells the node how to find the rooms this process owns.
func (n *Node) Attach(local func(gameID string) (room.Handle, bool), localIDs func() []string) {
	n.local, n.localIDs = local, localIDs
}

func (n *Node) channel(nodeID string) string  { return keyPrefix + "node:" + nodeID }
func (n *Node) aliveKey(nodeID string) string { return keyPrefix + "alive:" + nodeID }
func ownerKey(gameID string) string           { return keyPrefix + "owner:" + gameID }
func claimKey(gameID string) string           { return keyPrefix + "claim:" + gameID }
func inviteKey(code string) string            { return keyPrefix + "invite:" + strings.ToUpper(code) }

// Leave releases this node's games and liveness so peers adopt them
// immediately instead of after the TTL. Rooms keep running until the caller
// stops them; commands in flight may still complete locally.
func (n *Node) Leave(ctx context.Context) {
	n.once.Do(func() {
		keys := []string{n.aliveKey(n.ID)}
		for _, id := range n.localIDs() {
			keys = append(keys, ownerKey(id))
		}
		if err := n.bus.Del(ctx, keys...); err != nil {
			n.log.Warn("cluster: release keys", "err", err)
		}
		close(n.stop)
		n.cancel()
		n.wg.Wait()
		n.log.Info("cluster: left", "released", len(keys)-1)
	})
}

func (n *Node) writer() {
	defer n.wg.Done()
	for {
		select {
		case <-n.stop:
			return
		case o := <-n.out:
			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
			if err := n.bus.Publish(ctx, o.channel, o.payload); err != nil {
				n.log.Warn("cluster: publish", "err", err)
			}
			cancel()
		}
	}
}

func (n *Node) send(channel string, m message) {
	b, _ := json.Marshal(m)
	select {
	case n.out <- outbound{channel, b}:
	default:
		n.log.Warn("cluster: outbound queue full, dropping frame", "kind", m.Kind)
	}
}

// heartbeat refreshes liveness and reaps subscriptions to/from dead peers.
func (n *Node) heartbeat(every time.Duration) {
	defer n.wg.Done()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-n.stop:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
			if err := n.bus.Set(ctx, n.aliveKey(n.ID), "1", aliveTTL); err != nil {
				n.log.Warn("cluster: heartbeat", "err", err)
			}
			n.reapDeadPeers(ctx)
			cancel()
		}
	}
}

// PeerAlive reports whether nodeID heartbeated recently.
func (n *Node) PeerAlive(ctx context.Context, nodeID string) bool {
	if nodeID == n.ID {
		return true
	}
	_, ok, err := n.bus.Get(ctx, n.aliveKey(nodeID))
	return err == nil && ok
}

// Owner returns the live owner of a game.
func (n *Node) Owner(ctx context.Context, gameID string) (string, bool) {
	owner, ok, err := n.bus.Get(ctx, ownerKey(gameID))
	if err != nil || !ok || !n.PeerAlive(ctx, owner) {
		return "", false
	}
	return owner, true
}

// Claim makes this node the owner of a game unless a live peer already is.
// Two nodes racing for an orphaned game are serialised by a short lock.
func (n *Node) Claim(ctx context.Context, gameID string) bool {
	locked, err := n.bus.SetNX(ctx, claimKey(gameID), n.ID, claimTTL)
	if err != nil || !locked {
		return false
	}
	defer func() { _ = n.bus.Del(ctx, claimKey(gameID)) }()
	if owner, ok := n.Owner(ctx, gameID); ok && owner != n.ID {
		return false
	}
	if err := n.bus.Set(ctx, ownerKey(gameID), n.ID, 0); err != nil {
		n.log.Warn("cluster: claim", "game", gameID, "err", err)
		return false
	}
	return true
}

// Release forgets a finished game.
func (n *Node) Release(ctx context.Context, gameID, inviteCode string) {
	keys := []string{ownerKey(gameID)}
	if inviteCode != "" {
		keys = append(keys, inviteKey(inviteCode))
	}
	_ = n.bus.Del(ctx, keys...)
	n.mu.Lock()
	delete(n.proxies, gameID)
	n.mu.Unlock()
}

// SetInvite publishes an invite code so any node can resolve it.
func (n *Node) SetInvite(ctx context.Context, code, gameID string) {
	if code != "" {
		_ = n.bus.Set(ctx, inviteKey(code), gameID, 0)
	}
}

// LookupInvite resolves an invite code published by any node.
func (n *Node) LookupInvite(ctx context.Context, code string) (string, bool) {
	id, ok, err := n.bus.Get(ctx, inviteKey(code))
	return id, err == nil && ok
}

// Proxy returns a handle for a game owned by another node.
func (n *Node) Proxy(gameID, owner string) *RemoteRoom {
	n.mu.Lock()
	defer n.mu.Unlock()
	if p, ok := n.proxies[gameID]; ok {
		p.mu.Lock()
		p.owner = owner
		p.mu.Unlock()
		return p
	}
	p := &RemoteRoom{node: n, id: gameID, owner: owner, subs: map[room.Subscriber]string{}}
	n.proxies[gameID] = p
	return p
}

// ---- inbound -------------------------------------------------------------------------

func (n *Node) handle(payload []byte) {
	var m message
	if err := json.Unmarshal(payload, &m); err != nil {
		n.log.Warn("cluster: bad message", "err", err)
		return
	}
	switch m.Kind {
	case "rpc":
		go n.serve(m)
	case "reply":
		n.mu.Lock()
		ch, ok := n.pending[m.ID]
		delete(n.pending, m.ID)
		n.mu.Unlock()
		if ok {
			ch <- m
		}
	case "frame":
		n.mu.Lock()
		c, ok := n.conns[m.Conn]
		n.mu.Unlock()
		if ok && m.Env != nil {
			c.sub.Send(*m.Env)
		}
	}
}

// call performs one RPC against the owner of a game.
func (n *Node) call(owner, gameID, method string, args any, result any) error {
	a, err := json.Marshal(args)
	if err != nil {
		return err
	}
	id := ids.New()[:16]
	ch := make(chan message, 1)
	n.mu.Lock()
	n.pending[id] = ch
	n.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()
	b, _ := json.Marshal(message{Kind: "rpc", ID: id, From: n.ID, Game: gameID, Method: method, Args: a})
	if err := n.bus.Publish(ctx, n.channel(owner), b); err != nil {
		n.forget(id)
		n.met.ClusterRPC.WithLabelValues(method, "publish_error").Inc()
		return err
	}
	select {
	case reply := <-ch:
		if reply.Error != nil {
			n.met.ClusterRPC.WithLabelValues(method, "error").Inc()
			return &room.Error{Code: reply.Error.Code, Message: reply.Error.Message}
		}
		n.met.ClusterRPC.WithLabelValues(method, "ok").Inc()
		if result != nil && len(reply.Result) > 0 {
			return json.Unmarshal(reply.Result, result)
		}
		return nil
	case <-ctx.Done():
		n.forget(id)
		n.met.ClusterRPC.WithLabelValues(method, "timeout").Inc()
		return &room.Error{Code: "unavailable", Message: "the game's server did not answer"}
	case <-n.stop:
		n.forget(id)
		return errors.New("node stopped")
	}
}

func (n *Node) forget(id string) {
	n.mu.Lock()
	delete(n.pending, id)
	n.mu.Unlock()
}

// serve executes an RPC on a locally owned room and replies.
func (n *Node) serve(m message) {
	reply := message{Kind: "reply", ID: m.ID}
	res, err := n.dispatch(m)
	if err != nil {
		reply.Error = toRPCError(err)
	} else if res != nil {
		reply.Result, _ = json.Marshal(res)
	}
	n.send(n.channel(m.From), reply)
}

func toRPCError(err error) *rpcError {
	var ge *game.Error
	if errors.As(err, &ge) {
		return &rpcError{Code: ge.Code, Message: ge.Message}
	}
	var re *room.Error
	if errors.As(err, &re) {
		return &rpcError{Code: re.Code, Message: re.Message}
	}
	return &rpcError{Code: "error", Message: err.Error()}
}

type (
	argsUser  struct{ UserID string }
	argsJoin  struct{ User store.User }
	argsBot   struct{ UserID, Profile string }
	argsKick  struct{ UserID, PlayerID string }
	argsReady struct {
		UserID string
		Ready  bool
	}
	argsSub struct {
		Conn, UserID string
		LastSeq      int
	}
	argsUnsub   struct{ Conn string }
	argsCommand struct {
		UserID, Type string
		Payload      json.RawMessage
	}
	argsChat struct{ UserID, Text string }
)

func decodeArgs[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	err := json.Unmarshal(raw, &v)
	return v, err
}

func (n *Node) dispatch(m message) (any, error) {
	h, ok := n.local(m.Game)
	if !ok {
		return nil, &room.Error{Code: "not_owner", Message: "this node does not host that game"}
	}
	switch m.Method {
	case "info":
		return h.Info(), nil
	case "record":
		return h.Record(), nil
	case "join":
		a, err := decodeArgs[argsJoin](m.Args)
		if err != nil {
			return nil, err
		}
		return nil, h.Join(&a.User)
	case "leave":
		a, err := decodeArgs[argsUser](m.Args)
		if err != nil {
			return nil, err
		}
		return nil, h.Leave(a.UserID)
	case "addBot":
		a, err := decodeArgs[argsBot](m.Args)
		if err != nil {
			return nil, err
		}
		return nil, h.AddBot(a.UserID, a.Profile)
	case "kick":
		a, err := decodeArgs[argsKick](m.Args)
		if err != nil {
			return nil, err
		}
		return nil, h.Kick(a.UserID, a.PlayerID)
	case "ready":
		a, err := decodeArgs[argsReady](m.Args)
		if err != nil {
			return nil, err
		}
		h.SetReady(a.UserID, a.Ready)
		return nil, nil
	case "start":
		a, err := decodeArgs[argsUser](m.Args)
		if err != nil {
			return nil, err
		}
		return nil, h.StartGame(a.UserID)
	case "subscribe":
		a, err := decodeArgs[argsSub](m.Args)
		if err != nil {
			return nil, err
		}
		sub := &remoteSub{node: n, peer: m.From, conn: a.Conn, userID: a.UserID, game: m.Game}
		n.mu.Lock()
		n.remote[m.From+"|"+a.Conn] = sub
		n.mu.Unlock()
		h.Subscribe(sub, a.LastSeq)
		return nil, nil
	case "unsubscribe":
		a, err := decodeArgs[argsUnsub](m.Args)
		if err != nil {
			return nil, err
		}
		n.mu.Lock()
		sub, ok := n.remote[m.From+"|"+a.Conn]
		delete(n.remote, m.From+"|"+a.Conn)
		n.mu.Unlock()
		if ok {
			h.Unsubscribe(sub)
		}
		return nil, nil
	case "command":
		a, err := decodeArgs[argsCommand](m.Args)
		if err != nil {
			return nil, err
		}
		cmd, err := game.DecodeCommand(a.Type, a.Payload)
		if err != nil {
			return nil, &room.Error{Code: "bad_request", Message: err.Error()}
		}
		return nil, h.Command(a.UserID, cmd)
	case "forceEnd":
		return nil, h.ForceEnd()
	case "chat":
		a, err := decodeArgs[argsChat](m.Args)
		if err != nil {
			return nil, err
		}
		h.Chat(a.UserID, a.Text)
		return nil, nil
	}
	return nil, fmt.Errorf("unknown rpc method %q", m.Method)
}

// reapDeadPeers drops subscriptions that involve a node that stopped
// heartbeating: remote subscribers on our rooms are unsubscribed, and our
// clients of rooms that node owned are told to rejoin (the next JoinGame
// re-resolves the owner, adopting the game if nobody has).
func (n *Node) reapDeadPeers(ctx context.Context) {
	n.mu.Lock()
	peers := map[string]bool{}
	for _, s := range n.remote {
		peers[s.peer] = true
	}
	for _, p := range n.proxies {
		p.mu.Lock()
		peers[p.owner] = true
		p.mu.Unlock()
	}
	n.mu.Unlock()
	for peer := range peers {
		if n.PeerAlive(ctx, peer) {
			continue
		}
		n.log.Warn("cluster: peer is gone", "peer", peer)
		n.mu.Lock()
		var subs []*remoteSub
		for k, s := range n.remote {
			if s.peer == peer {
				subs = append(subs, s)
				delete(n.remote, k)
			}
		}
		var proxies []*RemoteRoom
		for id, p := range n.proxies {
			if p.owner == peer {
				proxies = append(proxies, p)
				delete(n.proxies, id)
			}
		}
		n.mu.Unlock()
		for _, s := range subs {
			if h, ok := n.local(s.game); ok {
				h.Unsubscribe(s)
			}
		}
		for _, p := range proxies {
			p.orphan()
		}
	}
}

// ---- owner side: a subscriber that lives on another node ------------------------------

type remoteSub struct {
	node               *Node
	peer, conn, userID string
	game               string
}

func (s *remoteSub) UserID() string { return s.userID }

func (s *remoteSub) Send(env protocol.Envelope) {
	s.node.send(s.node.channel(s.peer), message{Kind: "frame", Conn: s.conn, Env: &env})
}

// ---- caller side: the proxy ---------------------------------------------------------------

type proxyConn struct {
	sub  room.Subscriber
	room *RemoteRoom
}

// RemoteRoom implements room.Handle for a game owned by another node.
type RemoteRoom struct {
	node *Node
	id   string

	mu    sync.Mutex
	owner string
	subs  map[room.Subscriber]string // subscriber -> conn id
}

var _ room.Handle = (*RemoteRoom)(nil)

func (p *RemoteRoom) ownerID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.owner
}

// Owner is the node hosting the game.
func (p *RemoteRoom) Owner() string { return p.ownerID() }

func (p *RemoteRoom) call(method string, args any, result any) error {
	return p.node.call(p.ownerID(), p.id, method, args, result)
}

func (p *RemoteRoom) ID() string { return p.id }

func (p *RemoteRoom) Info() protocol.Lobby {
	var l protocol.Lobby
	if err := p.call("info", nil, &l); err != nil {
		p.node.log.Warn("cluster: info", "game", p.id, "err", err)
	}
	return l
}

func (p *RemoteRoom) Record() *store.GameRecord {
	var rec store.GameRecord
	if err := p.call("record", nil, &rec); err != nil {
		return nil
	}
	return &rec
}

func (p *RemoteRoom) Join(u *store.User) error { return p.call("join", argsJoin{User: *u}, nil) }
func (p *RemoteRoom) Leave(userID string) error {
	return p.call("leave", argsUser{UserID: userID}, nil)
}
func (p *RemoteRoom) AddBot(byUserID, profile string) error {
	return p.call("addBot", argsBot{UserID: byUserID, Profile: profile}, nil)
}
func (p *RemoteRoom) Kick(byUserID, playerID string) error {
	return p.call("kick", argsKick{UserID: byUserID, PlayerID: playerID}, nil)
}
func (p *RemoteRoom) SetReady(userID string, ready bool) {
	_ = p.call("ready", argsReady{UserID: userID, Ready: ready}, nil)
}
func (p *RemoteRoom) StartGame(byUserID string) error {
	return p.call("start", argsUser{UserID: byUserID}, nil)
}
func (p *RemoteRoom) ForceEnd() error { return p.call("forceEnd", nil, nil) }
func (p *RemoteRoom) Chat(userID, text string) {
	_ = p.call("chat", argsChat{UserID: userID, Text: text}, nil)
}

func (p *RemoteRoom) Command(userID string, cmd game.Command) error {
	payload, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	return p.call("command", argsCommand{UserID: userID, Type: cmd.CommandType(), Payload: payload}, nil)
}

// Subscribe registers the local subscriber with the owner; frames arrive on
// this node's channel tagged with the connection id.
func (p *RemoteRoom) Subscribe(sub room.Subscriber, lastSeq int) {
	conn := ids.New()[:16]
	p.node.mu.Lock()
	p.node.conns[conn] = &proxyConn{sub: sub, room: p}
	p.node.mu.Unlock()
	p.mu.Lock()
	p.subs[sub] = conn
	p.mu.Unlock()
	if err := p.call("subscribe", argsSub{Conn: conn, UserID: sub.UserID(), LastSeq: lastSeq}, nil); err != nil {
		p.drop(sub)
		sub.Send(protocol.MustEncode(protocol.SError, "", protocol.Error{Code: "unavailable", Message: "could not reach the game's server, please rejoin"}))
	}
}

func (p *RemoteRoom) Unsubscribe(sub room.Subscriber) {
	conn, ok := p.drop(sub)
	if !ok {
		return
	}
	go func() { _ = p.call("unsubscribe", argsUnsub{Conn: conn}, nil) }()
}

func (p *RemoteRoom) drop(sub room.Subscriber) (string, bool) {
	p.mu.Lock()
	conn, ok := p.subs[sub]
	delete(p.subs, sub)
	p.mu.Unlock()
	if ok {
		p.node.mu.Lock()
		delete(p.node.conns, conn)
		p.node.mu.Unlock()
	}
	return conn, ok
}

// orphan tells every local subscriber that the owner vanished.
func (p *RemoteRoom) orphan() {
	p.mu.Lock()
	subs := make([]room.Subscriber, 0, len(p.subs))
	conns := make([]string, 0, len(p.subs))
	for s, c := range p.subs {
		subs = append(subs, s)
		conns = append(conns, c)
	}
	p.subs = map[room.Subscriber]string{}
	p.mu.Unlock()
	p.node.mu.Lock()
	for _, c := range conns {
		delete(p.node.conns, c)
	}
	p.node.rejoined += len(subs)
	p.node.mu.Unlock()
	for _, s := range subs {
		s.Send(protocol.MustEncode(protocol.SError, "", protocol.Error{Code: "rejoin", Message: p.id}))
	}
}
