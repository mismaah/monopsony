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

// The deal waits for every connected human's client to report its assets
// loaded, then starts as soon as the last one is in.
func TestStartWaitsForAssets(t *testing.T) {
	r, err := New(newRecord(), store.NewMem(), Options{AssetGrace: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	for _, u := range []*store.User{{ID: "u1", Name: "Alice"}, {ID: "u2", Name: "Bob"}} {
		if err := r.Join(u); err != nil {
			t.Fatal(err)
		}
	}
	s1, s2 := &fakeSub{id: "u1"}, &fakeSub{id: "u2"}
	r.Subscribe(s1, 0)
	r.Subscribe(s2, 0)

	// Alice's client preloaded while she sat in the lobby; Bob's has not.
	r.AssetsReady("u1")
	waitFor(t, func() bool { return seatInfo(t, s1.last(protocol.SLobby), "u1").Loaded })
	if err := r.StartGame("u1"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return lobbyOf(t, s2.last(protocol.SLobby)).Starting })
	if r.Record().Status != store.StatusLobby {
		t.Fatal("dealt before every client reported in")
	}
	// No new seats while the table is being held.
	if err := r.AddBot("u1", "balanced"); err == nil {
		t.Fatal("bots must not join a starting game")
	}
	if err := r.StartGame("u1"); err == nil {
		t.Fatal("a second start must be refused")
	}

	r.AssetsReady("u2")
	waitFor(t, func() bool { return s1.count(protocol.SSnapshot) >= 1 && s2.count(protocol.SSnapshot) >= 1 })
	if r.Record().Status != store.StatusInProgress {
		t.Fatal("expected the game to be in progress")
	}
}

// A client that never reports (stuck download, wedged tab) delays the table
// by the grace period, no longer, and a disconnected one not at all.
func TestStartGiveUpsOnSlowClients(t *testing.T) {
	r, err := New(newRecord(), store.NewMem(), Options{AssetGrace: 80 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	for _, u := range []*store.User{{ID: "u1", Name: "Alice"}, {ID: "u2", Name: "Bob"}} {
		if err := r.Join(u); err != nil {
			t.Fatal(err)
		}
	}
	s1, s2 := &fakeSub{id: "u1"}, &fakeSub{id: "u2"}
	r.Subscribe(s1, 0)
	r.Subscribe(s2, 0)
	if err := r.StartGame("u1"); err != nil {
		t.Fatal(err)
	}
	if r.Record().Status != store.StatusLobby {
		t.Fatal("dealt without waiting at all")
	}
	waitFor(t, func() bool { return s1.count(protocol.SSnapshot) >= 1 })

	// Bob drops out of a second table mid-wait: nobody is watching his
	// screen, so the table deals at once instead of serving out the grace.
	r2, err := New(newRecord(), store.NewMem(), Options{AssetGrace: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Stop()
	for _, u := range []*store.User{{ID: "u1", Name: "Alice"}, {ID: "u2", Name: "Bob"}} {
		if err := r2.Join(u); err != nil {
			t.Fatal(err)
		}
	}
	a, b := &fakeSub{id: "u1"}, &fakeSub{id: "u2"}
	r2.Subscribe(a, 0)
	r2.Subscribe(b, 0)
	r2.AssetsReady("u1")
	if err := r2.StartGame("u1"); err != nil {
		t.Fatal(err)
	}
	r2.Unsubscribe(b)
	waitFor(t, func() bool { return a.count(protocol.SSnapshot) >= 1 })
}

// A zero grace (tests, headless play) deals straight away.
func TestStartWithoutGraceIsImmediate(t *testing.T) {
	r, err := New(newRecord(), store.NewMem(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	for _, u := range []*store.User{{ID: "u1", Name: "Alice"}, {ID: "u2", Name: "Bob"}} {
		if err := r.Join(u); err != nil {
			t.Fatal(err)
		}
	}
	s1 := &fakeSub{id: "u1"}
	r.Subscribe(s1, 0)
	if err := r.StartGame("u1"); err != nil {
		t.Fatal(err)
	}
	if r.Record().Status != store.StatusInProgress {
		t.Fatal("expected an immediate deal")
	}
}

func lobbyOf(t *testing.T, env *protocol.Envelope) protocol.Lobby {
	t.Helper()
	var l protocol.Lobby
	if env == nil {
		return l
	}
	if err := json.Unmarshal(env.P, &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func seatInfo(t *testing.T, env *protocol.Envelope, playerID string) protocol.SeatInfo {
	t.Helper()
	for _, s := range lobbyOf(t, env).Seats {
		if s.PlayerID == playerID {
			return s
		}
	}
	return protocol.SeatInfo{}
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

// A pending trade pauses the game: the recipient gets TradeTimeout to answer,
// the proposer cannot play on meanwhile, a lapsed offer is simply declined
// (not an AFK strike), and the proposer's turn clock resumes afterwards.
func TestTradePausesTurnClock(t *testing.T) {
	st := store.NewMem()
	r, err := New(newRecord(), st, Options{TurnTimeout: time.Second, TradeTimeout: 50 * time.Millisecond, MaxTimeouts: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	for _, u := range []*store.User{{ID: "u1", Name: "Alice"}, {ID: "u2", Name: "Bob"}} {
		if err := r.Join(u); err != nil {
			t.Fatal(err)
		}
	}
	s1 := &fakeSub{id: "u1"}
	r.Subscribe(s1, 0)
	if err := r.StartGame("u1"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s1.count(protocol.SSnapshot) >= 1 })

	// snap copies the state on the actor goroutine: the timer mutates the
	// live one, so the test must never read it directly.
	snap := func() (*game.State, time.Time) {
		v, _ := call(r, func() (struct {
			s game.State
			d time.Time
		}, error) {
			return struct {
				s game.State
				d time.Time
			}{*r.state, r.deadline}, nil
		})
		return &v.s, v.d
	}
	_, turnDeadline := snap()
	if turnDeadline.IsZero() {
		t.Fatal("expected a turn deadline")
	}

	proposedAt := time.Now()
	if err := r.Command("u1", &game.ProposeTrade{Base: game.Base{PlayerID: "u1"}, ToID: "u2", Give: game.TradeSide{Cash: 5}}); err != nil {
		t.Fatal(err)
	}
	s, tradeDeadline := snap()
	if s.Trade == nil {
		t.Fatal("expected a pending trade")
	}
	if left := tradeDeadline.Sub(proposedAt); left > 100*time.Millisecond {
		t.Fatalf("trade deadline should be ~TradeTimeout away, got %v", left)
	}
	// The proposer is frozen while the offer stands.
	if err := r.Command("u1", &game.RollDice{Base: game.Base{PlayerID: "u1"}}); err == nil {
		t.Fatal("proposer should not be able to roll during a pending trade")
	}

	// Recipient never answers: the offer lapses, play resumes with u1.
	waitFor(t, func() bool { s, _ := snap(); return s.Trade == nil })
	s, resumed := snap()
	if s.Turn.Phase != game.PhasePreRoll || s.CurrentPlayer().ID != "u1" {
		t.Fatalf("turn should resume for the proposer, got %s/%s", s.Turn.Phase, s.CurrentPlayer().ID)
	}
	if resumed.IsZero() || !resumed.After(tradeDeadline) {
		t.Fatalf("turn clock should be re-armed after the trade, got %v", resumed)
	}
	if seat := r.Record().Seats[1]; seat.IsBot {
		t.Fatal("a lapsed trade offer must not hand the seat to a bot")
	}
	if err := r.Command("u1", &game.RollDice{Base: game.Base{PlayerID: "u1"}}); err != nil {
		t.Fatal(err)
	}
}

// The host may remove a player mid-game: the seat carries on under a bot, the
// kicked user is told and loses the seat, and the game plays on. Anyone may
// surrender at any point and is out immediately.
func TestKickMidGameAndSurrender(t *testing.T) {
	st := store.NewMem()
	r, err := New(newRecord(), st, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	for _, u := range []*store.User{{ID: "u1", Name: "Host"}, {ID: "u2", Name: "Bob"}, {ID: "u3", Name: "Cara"}} {
		if err := r.Join(u); err != nil {
			t.Fatal(err)
		}
	}
	s1, s2, s3 := &fakeSub{id: "u1"}, &fakeSub{id: "u2"}, &fakeSub{id: "u3"}
	r.Subscribe(s1, 0)
	r.Subscribe(s2, 0)
	r.Subscribe(s3, 0)
	if err := r.StartGame("u1"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s2.count(protocol.SSnapshot) >= 1 })

	if err := r.Kick("u2", "u1"); err == nil {
		t.Fatal("only the host may kick")
	}
	if err := r.Kick("u1", "u1"); err == nil {
		t.Fatal("the host cannot kick themselves")
	}
	if err := r.Kick("u1", "nobody"); err == nil {
		t.Fatal("expected not_found")
	}
	// u1 (host) is mid-turn; kicking u2 hands u2's seat to a bot.
	if err := r.Kick("u1", "u2"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return s2.count(protocol.SKicked) == 1 })
	if s1.count(protocol.SKicked) != 0 || s3.count(protocol.SKicked) != 0 {
		t.Fatal("only the kicked user should be told")
	}
	var seat *store.SeatRecord
	for _, s := range r.Record().Seats {
		if s.PlayerID == "u2" {
			c := s
			seat = &c
		}
	}
	if seat == nil || !seat.IsBot || seat.UserID != "" {
		t.Fatalf("kicked seat should be a detached bot: %+v", seat)
	}
	if err := r.Command("u2", &game.Surrender{Base: game.Base{PlayerID: "u2"}}); err == nil {
		t.Fatal("a kicked user should no longer be seated")
	}
	if err := r.Kick("u1", "u2"); err == nil {
		t.Fatal("a bot seat cannot be kicked mid-game")
	}
	var up protocol.Update
	if err := json.Unmarshal(s2.last(protocol.SUpdate).P, &up); err != nil {
		t.Fatal(err)
	}
	if len(up.Actions) != 0 {
		t.Fatalf("kicked user should have no actions, got %v", up.Actions)
	}

	// Cara surrenders off-turn; the host is then last human standing against a bot.
	if err := r.Command("u3", &game.Surrender{Base: game.Base{PlayerID: "u3"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Command("u3", &game.Surrender{Base: game.Base{PlayerID: "u3"}}); err == nil {
		t.Fatal("surrendering twice should fail")
	}
	rec := r.Record()
	var s game.State
	_ = json.Unmarshal(rec.State, &s)
	s.Bind(rec.Config)
	if !s.PlayerByID("u3").Bankrupt || s.PlayerByID("u1").Bankrupt || s.Turn.Phase == game.PhaseGameOver {
		t.Fatalf("expected u3 out and the game continuing: %+v", s.Turn)
	}
	if err := game.CheckInvariants(&s); err != nil {
		t.Fatal(err)
	}
	// The host surrenders on their own turn: the bot is last standing.
	if err := r.Command("u1", &game.Surrender{Base: game.Base{PlayerID: "u1"}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.Record().Status == store.StatusFinished })
	if r.Record().WinnerID != "u2" {
		t.Fatalf("expected the bot in u2's seat to win, got %s", r.Record().WinnerID)
	}
}
