package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"monopsony/server/internal/game"
	"monopsony/server/internal/protocol"
	"monopsony/server/internal/room"
	"monopsony/server/internal/store"
)

// fakeRoom records calls and echoes frames to its subscribers.
type fakeRoom struct {
	mu   sync.Mutex
	subs map[room.Subscriber]bool
	log  []string
}

func newFakeRoom() *fakeRoom { return &fakeRoom{subs: map[room.Subscriber]bool{}} }

func (f *fakeRoom) note(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, s)
}
func (f *fakeRoom) ID() string { return "g1" }
func (f *fakeRoom) Info() protocol.Lobby {
	return protocol.Lobby{GameID: "g1", Name: "fake", Status: store.StatusLobby}
}
func (f *fakeRoom) Record() *store.GameRecord { return &store.GameRecord{ID: "g1", Name: "fake"} }
func (f *fakeRoom) Join(u *store.User) error {
	f.note("join:" + u.Name)
	return nil
}
func (f *fakeRoom) Leave(userID string) error             { f.note("leave:" + userID); return nil }
func (f *fakeRoom) AddBot(byUserID, profile string) error { f.note("bot:" + profile); return nil }
func (f *fakeRoom) Kick(byUserID, playerID string) error  { f.note("kick:" + playerID); return nil }
func (f *fakeRoom) SetReady(userID string, ready bool)    { f.note("ready") }
func (f *fakeRoom) StartGame(byUserID string) error {
	return &room.Error{Code: "not_enough_players", Message: "need 2"}
}
func (f *fakeRoom) ForceEnd() error { f.note("end"); return nil }
func (f *fakeRoom) Chat(userID, text string) {
	f.broadcast(protocol.MustEncode(protocol.SChat, "", protocol.ChatMessage{Text: text}))
}
func (f *fakeRoom) Command(userID string, cmd game.Command) error {
	f.note("cmd:" + cmd.CommandType() + ":" + cmd.Actor())
	return nil
}
func (f *fakeRoom) Subscribe(sub room.Subscriber, lastSeq int) {
	f.mu.Lock()
	f.subs[sub] = true
	f.mu.Unlock()
	sub.Send(protocol.MustEncode(protocol.SLobby, "", f.Info()))
}
func (f *fakeRoom) Unsubscribe(sub room.Subscriber) {
	f.mu.Lock()
	delete(f.subs, sub)
	f.mu.Unlock()
	f.note("unsub")
}
func (f *fakeRoom) broadcast(env protocol.Envelope) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for s := range f.subs {
		s.Send(env)
	}
}
func (f *fakeRoom) subCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

type recSub struct {
	mu   sync.Mutex
	msgs []protocol.Envelope
}

func (r *recSub) UserID() string { return "u1" }
func (r *recSub) Send(e protocol.Envelope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, e)
}
func (r *recSub) find(t string) *protocol.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.msgs {
		if r.msgs[i].T == t {
			return &r.msgs[i]
		}
	}
	return nil
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func twoNodes(t *testing.T, bus Bus, hb time.Duration) (*Node, *Node, *fakeRoom) {
	t.Helper()
	ctx := context.Background()
	a, err := Join(ctx, bus, Options{ID: "a", Heartbeat: hb})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Join(ctx, bus, Options{ID: "b", Heartbeat: hb})
	if err != nil {
		t.Fatal(err)
	}
	fr := newFakeRoom()
	a.Attach(func(id string) (room.Handle, bool) {
		if id == "g1" {
			return fr, true
		}
		return nil, false
	}, func() []string { return []string{"g1"} })
	if !a.Claim(ctx, "g1") {
		t.Fatal("a should claim g1")
	}
	t.Cleanup(func() { a.Leave(ctx); b.Leave(ctx) })
	return a, b, fr
}

func TestRPCAndFrames(t *testing.T) {
	ctx := context.Background()
	a, b, fr := twoNodes(t, NewMemBus(), time.Second)
	owner, ok := b.Owner(ctx, "g1")
	if !ok || owner != a.ID {
		t.Fatalf("owner = %q %v", owner, ok)
	}
	if b.Claim(ctx, "g1") {
		t.Fatal("b must not steal a live owner's game")
	}
	p := b.Proxy("g1", a.ID)
	if p.Info().Name != "fake" {
		t.Fatal("info rpc")
	}
	if err := p.Join(&store.User{ID: "u1", Name: "Ann"}); err != nil {
		t.Fatal(err)
	}
	var re *room.Error
	if err := p.StartGame("u1"); !errors.As(err, &re) || re.Code != "not_enough_players" {
		t.Fatalf("error codes must survive the wire: %v", err)
	}
	if err := p.Command("u1", game.RollDice{Base: game.Base{PlayerID: "u1"}}); err != nil {
		t.Fatal(err)
	}
	sub := &recSub{}
	p.Subscribe(sub, 0)
	waitFor(t, "lobby frame", func() bool { return sub.find(protocol.SLobby) != nil })
	p.Chat("u1", "hello")
	waitFor(t, "chat frame", func() bool { return sub.find(protocol.SChat) != nil })
	p.Unsubscribe(sub)
	waitFor(t, "unsubscribe", func() bool { return fr.subCount() == 0 })

	// An RPC for a game this node does not host is refused, not hung.
	if err := b.call(a.ID, "nope", "info", nil, nil); !errors.As(err, &re) || re.Code != "not_owner" {
		t.Fatalf("not_owner: %v", err)
	}
	// An RPC to a dead node times out instead of blocking forever.
	if err := b.call("ghost", "g1", "info", nil, nil); !errors.As(err, &re) || re.Code != "unavailable" {
		t.Fatalf("unavailable: %v", err)
	}
}

func TestDeadPeerIsReaped(t *testing.T) {
	ctx := context.Background()
	bus := NewMemBus()
	a, b, fr := twoNodes(t, bus, 30*time.Millisecond)
	p := b.Proxy("g1", a.ID)
	sub := &recSub{}
	p.Subscribe(sub, 0)
	waitFor(t, "subscribe", func() bool { return fr.subCount() == 1 })

	// Simulate a crash: a stops heartbeating, but its owner key lingers
	// (a graceful Leave would have deleted it; a crash does not).
	a.Leave(ctx)
	_ = bus.Set(ctx, ownerKey("g1"), a.ID, 0)
	if _, ok := b.Owner(ctx, "g1"); ok {
		t.Fatal("dead owner must not count")
	}
	waitFor(t, "rejoin frame", func() bool {
		e := sub.find(protocol.SError)
		return e != nil && string(e.P) == `{"code":"rejoin","message":"g1"}`
	})
	// b can now take the game over.
	if !b.Claim(ctx, "g1") {
		t.Fatal("b should claim the orphaned game")
	}
	owner, _ := b.Owner(ctx, "g1")
	if owner != b.ID {
		t.Fatalf("owner after claim = %q", owner)
	}
}
