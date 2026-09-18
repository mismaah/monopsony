package room

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"monopsony/server/internal/game"
	"monopsony/server/internal/protocol"
	"monopsony/server/internal/store"
)

type fakeSub struct {
	id   string
	mu   sync.Mutex
	msgs []protocol.Envelope
}

func (f *fakeSub) UserID() string { return f.id }
func (f *fakeSub) Send(env protocol.Envelope) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, env)
}
func (f *fakeSub) count(t string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.msgs {
		if m.T == t {
			n++
		}
	}
	return n
}
func (f *fakeSub) last(t string) *protocol.Envelope {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.msgs) - 1; i >= 0; i-- {
		if f.msgs[i].T == t {
			return &f.msgs[i]
		}
	}
	return nil
}

func newRecord() *store.GameRecord {
	return &store.GameRecord{
		ID: "g1", Name: "test", Status: store.StatusLobby, HostID: "u1", Visibility: "private",
		MaxPlayers: 4, TurnSeconds: 0, ConfigID: "classic", Config: game.ClassicConfig(), CreatedAt: time.Now(),
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestLobbyToGameAndCommands(t *testing.T) {
	st := store.NewMem()
	r, err := New(newRecord(), st, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	u1 := &store.User{ID: "u1", Name: "Alice"}
	u2 := &store.User{ID: "u2", Name: "Bob"}
	if err := r.Join(u1); err != nil {
		t.Fatal(err)
	}
	if err := r.Join(u2); err != nil {
		t.Fatal(err)
	}
	if err := r.StartGame("u2"); err == nil {
		t.Fatal("non-host should not start")
	}
	s1, s2 := &fakeSub{id: "u1"}, &fakeSub{id: "u2"}
	r.Subscribe(s1, 0)
	r.Subscribe(s2, 0)
	if err := r.StartGame("u1"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s1.count(protocol.SSnapshot) >= 1 && s2.count(protocol.SSnapshot) >= 1 })

	// Wrong player is rejected; right player rolls.
	if err := r.Command("u2", &game.RollDice{Base: game.Base{PlayerID: "u2"}}); err == nil {
		t.Fatal("expected not_your_turn")
	}
	if err := r.Command("u1", &game.RollDice{Base: game.Base{PlayerID: "u2"}}); err == nil {
		t.Fatal("expected forbidden: acting for another player")
	}
	if err := r.Command("u1", &game.RollDice{Base: game.Base{PlayerID: "u1"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s2.count(protocol.SEvent) >= 2 })
	if s1.last(protocol.SUpdate) == nil {
		t.Fatal("expected Update after command")
	}

	// Persisted: snapshot + events in the store.
	rec, err := st.GetGame(context.Background(), "g1")
	if err != nil || rec.Status != store.StatusInProgress || len(rec.State) == 0 || rec.Seq == 0 {
		t.Fatalf("record not persisted: %v %+v", err, rec)
	}
	evs, _ := st.GetEvents(context.Background(), "g1", 0)
	if len(evs) != rec.Seq {
		t.Fatalf("expected %d events, got %d", rec.Seq, len(evs))
	}

	// Reconnect with lastSeq replays the gap then snapshots.
	s3 := &fakeSub{id: "u2"}
	r.Subscribe(s3, 2)
	waitFor(t, func() bool { return s3.count(protocol.SSnapshot) == 1 })
	if s3.count(protocol.SEvent) != rec.Seq-2 {
		t.Fatalf("expected %d replayed events, got %d", rec.Seq-2, s3.count(protocol.SEvent))
	}

	// Restart: a new room from the stored record continues the same game.
	r.Stop()
	r2, err := New(rec, st, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Stop()
	s4 := &fakeSub{id: "u1"}
	r2.Subscribe(s4, 0)
	waitFor(t, func() bool { return s4.count(protocol.SSnapshot) == 1 })
	var snap protocol.Snapshot
	if err := json.Unmarshal(s4.last(protocol.SSnapshot).P, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.State.Seq != rec.Seq || !snap.State.Turn.Rolled {
		t.Fatalf("restored state mismatch: seq=%d rolled=%v", snap.State.Seq, snap.State.Turn.Rolled)
	}
}

func TestBotsPlayToCompletion(t *testing.T) {
	st := store.NewMem()
	rec := newRecord()
	rec.Config.Rules.TurnLimit = 60
	r, err := New(rec, st, Options{})
	if err != nil {
		t.Fatal(err)
	}
	host := &store.User{ID: "u1", Name: "Host"}
	if err := r.Join(host); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"balanced", "aggressive", "cautious"} {
		if err := r.AddBot("u1", p); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.AddBot("u1", "balanced"); err == nil {
		t.Fatal("expected room_full")
	}
	// Leaving and re-joining keeps the host seat.
	if err := r.Leave("u1"); err != nil {
		t.Fatal(err)
	}
	if err := r.Join(host); err != nil {
		t.Fatal(err)
	}
	lobbyRec := r.Record()
	r.Stop()

	// Reopen with a tiny human timeout: the host never acts, so the safe
	// default plays for them and after MaxTimeouts a bot takes the seat.
	r, err = New(lobbyRec, st, Options{BotDelay: 0, TurnTimeout: 20 * time.Millisecond, MaxTimeouts: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	if err := r.StartGame("u1"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.Record().Status == store.StatusFinished })
	final := r.Record()
	if final.WinnerID == "" {
		t.Fatal("expected a winner")
	}
	var s game.State
	_ = json.Unmarshal(final.State, &s)
	s.Bind(final.Config)
	if err := game.CheckInvariants(&s); err != nil {
		t.Fatal(err)
	}
	for _, seat := range final.Seats {
		if seat.UserID == "u1" && !seat.IsBot {
			t.Fatal("expected the idle human seat to be handed to a bot")
		}
	}
}
