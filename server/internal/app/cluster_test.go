package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"monopsony/server/internal/cluster"
	"monopsony/server/internal/protocol"
	"monopsony/server/internal/store"
)

// newNode builds one app instance on a shared store + bus, as two server
// processes behind a load balancer would be.
func newNode(t *testing.T, st store.Store, bus cluster.Bus, id string) (*httptest.Server, *App) {
	t.Helper()
	a, err := New(context.Background(), st, Config{JWTSecret: "test", Origins: []string{"http://example.test"}, Cluster: bus, NodeID: id})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return srv, a
}

type wsClient struct {
	t    *testing.T
	ctx  context.Context
	conn *websocket.Conn
}

func dialWS(t *testing.T, ctx context.Context, base, token string) *wsClient {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(base, "http") + "/ws?token=" + token
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"http://example.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })
	return &wsClient{t: t, ctx: ctx, conn: conn}
}

func (c *wsClient) write(env protocol.Envelope) {
	if err := wsjson.Write(c.ctx, c.conn, env); err != nil {
		c.t.Fatal(err)
	}
}

// read returns the next frame of type want, failing on Error frames unless
// that is what is wanted.
func (c *wsClient) read(want string) protocol.Envelope {
	c.t.Helper()
	for {
		var env protocol.Envelope
		if err := wsjson.Read(c.ctx, c.conn, &env); err != nil {
			c.t.Fatalf("read (waiting for %s): %v", want, err)
		}
		if env.T == want {
			return env
		}
		if env.T == protocol.SError {
			c.t.Fatalf("server error while waiting for %s: %s", want, env.P)
		}
	}
}

func TestClusterRoomsAreReachableFromAnyNode(t *testing.T) {
	st := store.NewMem()
	bus := cluster.NewMemBus()
	srvA, appA := newNode(t, st, bus, "node-a")
	srvB, appB := newNode(t, st, bus, "node-b")

	host := guest(t, srvA.URL, "Host")
	joiner := guest(t, srvB.URL, "Joiner")

	// The game is created (and therefore hosted) on A.
	out := host.call("POST", "/api/games", map[string]any{"name": "cluster", "turnSeconds": 30}, 201)
	gameID := out["game"].(map[string]any)["gameId"].(string)
	if _, local := appA.Lobby.Get(gameID); !local {
		t.Fatal("A should host the game")
	}
	if h, ok := appB.Lobby.Get(gameID); !ok {
		t.Fatal("B should resolve the game")
	} else if _, isProxy := h.(*cluster.RemoteRoom); !isProxy {
		t.Fatalf("B should hold a proxy, got %T", h)
	}

	// B lists A's public lobby and lets its user join through the proxy.
	list := joiner.call("GET", "/api/games", nil, 200)["games"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["node"] != "node-a" {
		t.Fatalf("cross-node listing: %v", list)
	}
	joined := joiner.call("POST", "/api/games/"+gameID+"/join", nil, 200)
	if seats := joined["game"].(map[string]any)["seats"].([]any); len(seats) != 2 {
		t.Fatalf("join via proxy: %v", seats)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// The joiner's socket is on B; frames are relayed from A's room.
	wsB := dialWS(t, ctx, srvB.URL, joiner.token)
	wsB.read(protocol.SWelcome)
	wsB.write(protocol.MustEncode(protocol.CJoinGame, "j", protocol.JoinGame{GameID: gameID}))
	wsB.read(protocol.SLobby)

	host.call("POST", "/api/games/"+gameID+"/start", nil, 200)
	snapEnv := wsB.read(protocol.SSnapshot)
	var snap protocol.Snapshot
	_ = json.Unmarshal(snapEnv.P, &snap)
	if len(snap.State.Players) != 2 {
		t.Fatalf("snapshot via relay: %d players", len(snap.State.Players))
	}

	// Commands from B are forwarded to A and rejected/acked with the ref.
	wsB.write(protocol.MustEncode(protocol.CCommand, "c1", protocol.Command{GameID: gameID, Type: "RollDice"}))
	errEnv := wsB.read(protocol.SError)
	if errEnv.Ref != "c1" {
		t.Fatalf("expected not-your-turn error with ref c1, got %s %s", errEnv.Ref, errEnv.P)
	}
	// Host rolls on A; B's client sees the events flow through the relay.
	wsA := dialWS(t, ctx, srvA.URL, host.token)
	wsA.read(protocol.SWelcome)
	wsA.write(protocol.MustEncode(protocol.CJoinGame, "j", protocol.JoinGame{GameID: gameID}))
	wsA.read(protocol.SSnapshot)
	wsA.write(protocol.MustEncode(protocol.CCommand, "c2", protocol.Command{GameID: gameID, Type: "RollDice"}))
	wsA.read(protocol.SAck)
	ev := wsB.read(protocol.SEvent)
	var e protocol.Event
	_ = json.Unmarshal(ev.P, &e)
	if e.Type != "DiceRolled" {
		t.Fatalf("relayed event: %s", e.Type)
	}
	upd := wsB.read(protocol.SUpdate)
	var u protocol.Update
	_ = json.Unmarshal(upd.P, &u)
	seqBefore := u.Seq

	// Chat is relayed too.
	wsB.write(protocol.MustEncode(protocol.CChat, "", protocol.Chat{GameID: gameID, Text: "hi from B"}))
	chat := wsA.read(protocol.SChat)
	if !strings.Contains(string(chat.P), "hi from B") {
		t.Fatalf("chat relay: %s", chat.P)
	}

	// A goes away gracefully: it releases its games. B adopts the game from
	// the store on the next lookup and play continues where it left off.
	appA.Shutdown(context.Background())
	h, ok := appB.Lobby.Get(gameID)
	if !ok {
		t.Fatal("B should adopt the orphaned game")
	}
	if _, isProxy := h.(*cluster.RemoteRoom); isProxy {
		t.Fatal("B should now host the game locally")
	}
	rec := h.Record()
	if rec == nil || rec.Status != store.StatusInProgress || rec.Seq != seqBefore {
		t.Fatalf("adopted record: %+v", rec)
	}
	wsB2 := dialWS(t, ctx, srvB.URL, joiner.token)
	wsB2.read(protocol.SWelcome)
	wsB2.write(protocol.MustEncode(protocol.CJoinGame, "j", protocol.JoinGame{GameID: gameID, LastSeq: seqBefore}))
	snapEnv = wsB2.read(protocol.SSnapshot)
	_ = json.Unmarshal(snapEnv.P, &snap)
	if snap.State.Seq != seqBefore {
		t.Fatalf("adopted state seq %d, want %d", snap.State.Seq, seqBefore)
	}
	if got := appB.Lobby.ListAll(); len(got) != 1 || got[0].Node != "node-b" {
		t.Fatalf("after adoption ListAll = %+v", got)
	}
}

func TestClusterRestoreSplitsGames(t *testing.T) {
	st := store.NewMem()
	bus := cluster.NewMemBus()
	// Seed three lobbies on a first node, then leave it.
	srv0, app0 := newNode(t, st, bus, "seed")
	host := guest(t, srv0.URL, "Host")
	u, _ := app0.Store.GetUser(context.Background(), host.user["id"].(string))
	u.Tier = "premium"
	_ = app0.Store.UpdateUser(context.Background(), u)
	for i := 0; i < 3; i++ {
		host.call("POST", "/api/games", map[string]any{"visibility": "private"}, 201)
	}
	app0.Shutdown(context.Background())

	// Two fresh nodes boot together: every game ends up on exactly one.
	_, a := newNode(t, st, bus, "n1")
	_, b := newNode(t, st, bus, "n2")
	na, nb := len(a.Lobby.Local()), len(b.Lobby.Local())
	if na+nb != 3 || na != 3 {
		// n1 restored first and claimed everything; n2 skipped them all.
		t.Fatalf("games split n1=%d n2=%d", na, nb)
	}
	// n2 resolves n1's games to proxies, and invite codes work cluster-wide.
	for _, r := range a.Lobby.Local() {
		code := r.Record().InviteCode
		h, ok := b.Lobby.ByInvite(code)
		if !ok {
			t.Fatalf("invite %s not resolvable from n2", code)
		}
		if _, isProxy := h.(*cluster.RemoteRoom); !isProxy {
			t.Fatalf("expected proxy for %s", r.ID())
		}
	}
}
