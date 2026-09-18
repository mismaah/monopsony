package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"monopsony/server/internal/protocol"
	"monopsony/server/internal/store"
)

type client struct {
	t     *testing.T
	base  string
	token string
	user  map[string]any
}

func (c *client) call(method, path string, body any, want int) map[string]any {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, c.base+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: want %d got %d: %v", method, path, want, res.StatusCode, out)
	}
	return out
}

func guest(t *testing.T, base, name string) *client {
	c := &client{t: t, base: base}
	out := c.call("POST", "/api/auth/guest", map[string]string{"name": name}, 200)
	c.token = out["accessToken"].(string)
	c.user = out["user"].(map[string]any)
	return c
}

func newServer(t *testing.T) (*httptest.Server, *App) {
	t.Helper()
	a, err := New(context.Background(), store.NewMem(), Config{JWTSecret: "test", Origins: []string{"http://example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return srv, a
}

func TestAuthFlow(t *testing.T) {
	srv, _ := newServer(t)
	c := &client{t: t, base: srv.URL}
	c.call("POST", "/api/auth/register", map[string]string{"email": "a@example.com", "password": "hunter22!", "name": "Alice"}, 200)
	c.call("POST", "/api/auth/register", map[string]string{"email": "a@example.com", "password": "hunter22!", "name": "Alice"}, 409)
	c.call("POST", "/api/auth/login", map[string]string{"email": "a@example.com", "password": "wrong"}, 401)
	out := c.call("POST", "/api/auth/login", map[string]string{"email": "a@example.com", "password": "hunter22!"}, 200)
	c.token = out["accessToken"].(string)
	me := c.call("GET", "/api/me", nil, 200)
	if me["user"].(map[string]any)["name"] != "Alice" || me["caps"].(map[string]any)["tier"] != "free" {
		t.Fatalf("me: %v", me)
	}
	c.token = "garbage"
	c.call("GET", "/api/me", nil, 401)
}

func TestEntitlementsGateLobby(t *testing.T) {
	srv, a := newServer(t)
	host := guest(t, srv.URL, "Host")
	// Free tier: no private rooms, max 4 players.
	host.call("POST", "/api/games", map[string]any{"visibility": "private"}, 402)
	host.call("POST", "/api/games", map[string]any{"maxPlayers": 6}, 402)
	host.call("POST", "/api/games", map[string]any{"rules": map[string]any{"freeParkingJackpot": true}}, 402)
	// Upgrade the user and retry.
	u, _ := a.Store.GetUser(context.Background(), host.user["id"].(string))
	u.Tier = "premium"
	_ = a.Store.UpdateUser(context.Background(), u)
	out := host.call("POST", "/api/games", map[string]any{"visibility": "private", "maxPlayers": 6, "rules": map[string]any{"freeParkingJackpot": true, "auctionsEnabled": true}}, 201)
	g := out["game"].(map[string]any)
	if g["inviteCode"] == "" || g["maxPlayers"].(float64) != 6 || g["rules"].(map[string]any)["freeParkingJackpot"] != true {
		t.Fatalf("premium create: %v", g)
	}
	// A free user cannot join a 6-player private room.
	joiner := guest(t, srv.URL, "Joiner")
	joiner.call("POST", "/api/games/join-code", map[string]string{"code": g["inviteCode"].(string)}, 402)
	// Free user limited to 1 concurrent game.
	joiner.call("POST", "/api/games", map[string]any{}, 201)
	joiner.call("POST", "/api/games", map[string]any{}, 402)
}

func TestPlayOverWebSocket(t *testing.T) {
	srv, _ := newServer(t)
	host := guest(t, srv.URL, "Host")
	out := host.call("POST", "/api/games", map[string]any{"name": "ws test", "turnSeconds": 30}, 201)
	gameID := out["game"].(map[string]any)["gameId"].(string)
	host.call("POST", "/api/games/"+gameID+"/bots", map[string]string{"profile": "balanced"}, 200)
	list := host.call("GET", "/api/games", nil, 200)
	if n := len(list["games"].([]any)); n != 1 {
		t.Fatalf("expected 1 public lobby, got %d", n)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?token=" + host.token
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"http://example.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	read := func(want string) protocol.Envelope {
		t.Helper()
		for {
			var env protocol.Envelope
			if err := wsjson.Read(ctx, conn, &env); err != nil {
				t.Fatalf("read (waiting for %s): %v", want, err)
			}
			if env.T == want {
				return env
			}
			if env.T == protocol.SError {
				t.Fatalf("server error while waiting for %s: %s", want, env.P)
			}
		}
	}
	read(protocol.SWelcome)
	_ = wsjson.Write(ctx, conn, protocol.MustEncode(protocol.CJoinGame, "j1", protocol.JoinGame{GameID: gameID}))
	read(protocol.SLobby)

	host.call("POST", "/api/games/"+gameID+"/start", nil, 200)
	snapEnv := read(protocol.SSnapshot)
	var snap protocol.Snapshot
	_ = json.Unmarshal(snapEnv.P, &snap)
	if len(snap.State.Players) != 2 || snap.Deadline == 0 {
		t.Fatalf("snapshot: players=%d deadline=%d", len(snap.State.Players), snap.Deadline)
	}
	hasRoll := false
	for _, a := range snap.Legal {
		if a.Type == "RollDice" {
			hasRoll = true
		}
	}
	if !hasRoll {
		t.Fatalf("host should be able to roll: %v", snap.Legal)
	}

	// The client never names the actor; the server injects it.
	_ = wsjson.Write(ctx, conn, protocol.MustEncode(protocol.CCommand, "c1", protocol.Command{GameID: gameID, Type: "RollDice"}))
	// Events for the command are broadcast before its Ack arrives.
	var types []string
	for {
		var env protocol.Envelope
		if err := wsjson.Read(ctx, conn, &env); err != nil {
			t.Fatal(err)
		}
		if env.T == protocol.SEvent {
			var e protocol.Event
			_ = json.Unmarshal(env.P, &e)
			types = append(types, e.Type)
		}
		if env.T == protocol.SAck {
			if env.Ref != "c1" {
				t.Fatalf("ack ref %q", env.Ref)
			}
			break
		}
		if env.T == protocol.SError {
			t.Fatalf("command rejected: %s", env.P)
		}
	}
	if len(types) == 0 || types[0] != "DiceRolled" {
		t.Fatalf("expected DiceRolled first, got %v", types)
	}
	// An illegal command returns an Error with the ref.
	_ = wsjson.Write(ctx, conn, protocol.MustEncode(protocol.CCommand, "c2", protocol.Command{GameID: gameID, Type: "PlaceBid", Payload: json.RawMessage(`{"amount":5}`)}))
	errEnv := read(protocol.SError)
	if errEnv.Ref != "c2" {
		t.Fatalf("error ref %q: %s", errEnv.Ref, errEnv.P)
	}
}

func TestShopSubscribeAndLoadout(t *testing.T) {
	srv, _ := newServer(t)
	c := &client{t: t, base: srv.URL}
	out := c.call("POST", "/api/auth/register", map[string]string{"email": "shop@example.com", "password": "hunter22!", "name": "Shopper"}, 200)
	c.token = out["accessToken"].(string)

	cat := c.call("GET", "/api/shop/catalog", nil, 200)
	items := cat["items"].([]any)
	if len(items) < 5 {
		t.Fatalf("catalog too small: %d", len(items))
	}
	// The seeded token catalog: five free (owned by everyone), five premium
	// (tier-locked until subscribed or granted), plus one-off purchases.
	var freeTokens, premiumTokens int
	for _, raw := range items {
		it := raw.(map[string]any)
		if it["slot"] != "token" {
			continue
		}
		switch {
		case it["tierRequired"] == "premium":
			premiumTokens++
			if it["locked"] != "tier" || it["owned"] == true {
				t.Fatalf("premium token should be tier-locked for a free user: %v", it)
			}
		case it["priceCents"].(float64) == 0:
			freeTokens++
			if it["owned"] != true || it["locked"] != nil {
				t.Fatalf("free token should be owned and unlocked: %v", it)
			}
		}
	}
	if freeTokens != 5 || premiumTokens != 5 {
		t.Fatalf("expected 5 free + 5 premium tokens, got %d + %d", freeTokens, premiumTokens)
	}
	// Free item equips; premium-only board is locked on the free tier.
	c.call("PUT", "/api/me/loadout", map[string]string{"slot": "token", "itemId": "token.cone"}, 200)
	c.call("PUT", "/api/me/loadout", map[string]string{"slot": "board", "itemId": "board.midnight"}, 400)
	c.call("PUT", "/api/me/loadout", map[string]string{"slot": "token", "itemId": "token.gem"}, 400) // not owned

	// Subscribe through the fake provider: follow the checkout URL.
	sub := c.call("POST", "/api/billing/subscribe", map[string]string{"tier": "premium"}, 200)
	url := sub["url"].(string)
	url = srv.URL + url[len("http://localhost:5173"):]
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noRedirect.Get(url)
	if err != nil || res.StatusCode != 302 {
		t.Fatalf("fake checkout completion: %v %d", err, res.StatusCode)
	}
	me := c.call("GET", "/api/me", nil, 200)
	if me["caps"].(map[string]any)["tier"] != "premium" {
		t.Fatalf("expected premium after checkout: %v", me["caps"])
	}
	c.call("PUT", "/api/me/loadout", map[string]string{"slot": "board", "itemId": "board.midnight"}, 200)
	// Premium tokens come with the plan: no purchase step.
	c.call("PUT", "/api/me/loadout", map[string]string{"slot": "token", "itemId": "token.crown"}, 200)
	for _, raw := range c.call("GET", "/api/shop/catalog", nil, 200)["items"].([]any) {
		if it := raw.(map[string]any); it["id"] == "token.crown" && (it["owned"] != true || it["locked"] != nil) {
			t.Fatalf("premium token should be owned on the premium plan: %v", it)
		}
	}

	// Buy a paid token and equip it.
	buy := c.call("POST", "/api/shop/checkout", map[string]string{"itemId": "token.gem"}, 200)
	url = srv.URL + buy["url"].(string)[len("http://localhost:5173"):]
	if res, _ := noRedirect.Get(url); res.StatusCode != 302 {
		t.Fatalf("purchase completion: %d", res.StatusCode)
	}
	c.call("PUT", "/api/me/loadout", map[string]string{"slot": "token", "itemId": "token.gem"}, 200)
	info := c.call("GET", "/api/me/billing", nil, 200)
	if len(info["purchases"].([]any)) != 1 || info["subscription"].(map[string]any)["status"] != "active" {
		t.Fatalf("billing info: %v", info)
	}

	// The loadout shows up on the seat in a room.
	g := c.call("POST", "/api/games", map[string]any{}, 201)["game"].(map[string]any)
	seat := g["seats"].([]any)[0].(map[string]any)
	lo, _ := seat["loadout"].(map[string]any)
	if lo["token"] != "token.gem" || lo["board"] != "board.midnight" {
		t.Fatalf("seat loadout: %v", seat)
	}

	// Guests cannot buy.
	guest := guest(t, srv.URL, "Cheapskate")
	guest.call("POST", "/api/shop/checkout", map[string]string{"itemId": "token.gem"}, 403)
}

func TestAdminAPI(t *testing.T) {
	a, err := New(context.Background(), store.NewMem(), Config{JWTSecret: "test", AdminEmails: []string{"Admin@Example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	admin := &client{t: t, base: srv.URL}
	out := admin.call("POST", "/api/auth/register", map[string]string{"email": "admin@example.com", "password": "hunter22!", "name": "Root"}, 200)
	admin.token = out["accessToken"].(string)
	player := guest(t, srv.URL, "Player")

	// Role gate.
	player.call("GET", "/admin/api/configs", nil, 403)
	if admin.call("GET", "/admin/api/me", nil, 200)["user"].(map[string]any)["role"] != "admin" {
		t.Fatal("admin email should be promoted")
	}

	// Configs: publish a renamed version; new games pick it up, old ones don't.
	pub := admin.call("GET", "/admin/api/configs", nil, 200)["configs"].([]any)
	if len(pub) != 1 {
		t.Fatalf("expected 1 seeded config, got %d", len(pub))
	}
	before := player.call("POST", "/api/games", map[string]any{}, 201)["game"].(map[string]any)
	cfgRes := admin.call("GET", "/admin/api/configs/"+pub[0].(map[string]any)["id"].(string), nil, 200)
	cfg := cfgRes["config"].(map[string]any)["config"].(map[string]any)
	spaces := cfg["spaces"].([]any)
	spaces[39].(map[string]any)["name"] = "Promenade"
	cfg["rules"].(map[string]any)["startCash"] = 2000.0
	bad := map[string]any{"name": "broken", "config": map[string]any{"spaces": spaces[:5]}}
	admin.call("POST", "/admin/api/configs", bad, 400)
	created := admin.call("POST", "/admin/api/configs", map[string]any{"name": "v2 promenade", "config": cfg, "publish": true}, 201)["config"].(map[string]any)
	if created["version"].(float64) != 2 || created["published"] != true {
		t.Fatalf("created: %v", created)
	}
	after := player.call("POST", "/api/games", map[string]any{}, 402) // free tier: 1 active game
	_ = after
	premium := guest(t, srv.URL, "Rich")
	admin.call("PATCH", "/admin/api/users/"+premium.user["id"].(string), map[string]any{"tier": "premium"}, 200)
	g2 := premium.call("POST", "/api/games", map[string]any{}, 201)["game"].(map[string]any)
	if g2["configId"] == before["configId"] || g2["rules"].(map[string]any)["startCash"].(float64) != 2000 {
		t.Fatalf("new game should use the published v2 config: %v", g2["rules"])
	}
	if before["configId"] != pub[0].(map[string]any)["id"] {
		t.Fatal("existing game must keep its pinned config")
	}

	// Plans: raise the free player cap and watch the lobby honour it.
	plans := admin.call("GET", "/admin/api/plans", nil, 200)["plans"].([]any)
	var free map[string]any
	for _, p := range plans {
		if p.(map[string]any)["tier"] == "free" {
			free = p.(map[string]any)
		}
	}
	free["caps"].(map[string]any)["maxConcurrentGames"] = 3.0
	admin.call("PUT", "/admin/api/plans/free", free, 200)
	player.call("POST", "/api/games", map[string]any{}, 201)

	// Users: search, ban, grant cosmetic; banned user is locked out.
	users := admin.call("GET", "/admin/api/users?q=player", nil, 200)["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("user search: %v", users)
	}
	admin.call("POST", "/admin/api/users/"+player.user["id"].(string)+"/grant", map[string]string{"cosmeticId": "token.gem"}, 204)
	admin.call("POST", "/admin/api/users/"+player.user["id"].(string)+"/grant", map[string]string{"cosmeticId": "nope"}, 404)
	player.call("PUT", "/api/me/loadout", map[string]string{"slot": "token", "itemId": "token.gem"}, 200)
	// Granting a premium-tier item unlocks it for a free player too.
	player.call("PUT", "/api/me/loadout", map[string]string{"slot": "token", "itemId": "token.rocket"}, 400)
	admin.call("POST", "/admin/api/users/"+player.user["id"].(string)+"/grant", map[string]string{"cosmeticId": "token.rocket"}, 204)
	player.call("PUT", "/api/me/loadout", map[string]string{"slot": "token", "itemId": "token.rocket"}, 200)
	for _, raw := range player.call("GET", "/api/shop/catalog", nil, 200)["items"].([]any) {
		if it := raw.(map[string]any); it["id"] == "token.rocket" && (it["owned"] != true || it["locked"] != nil || it["equipped"] != true) {
			t.Fatalf("granted premium token should be owned, unlocked and equipped: %v", it)
		}
	}
	admin.call("PATCH", "/admin/api/users/"+admin.call("GET", "/admin/api/me", nil, 200)["user"].(map[string]any)["id"].(string), map[string]any{"banned": true}, 400)
	admin.call("PATCH", "/admin/api/users/"+player.user["id"].(string), map[string]any{"banned": true}, 200)
	player.call("GET", "/api/me", nil, 401)

	// Cosmetics upsert + rooms + audit.
	admin.call("PUT", "/admin/api/cosmetics/token.neon", map[string]any{"slot": "token", "name": "Neon", "priceCents": 499, "manifest": map[string]any{"builtin": "token.sphere", "color": "#0ff"}, "enabled": true}, 200)
	admin.call("PUT", "/admin/api/cosmetics/x", map[string]any{"slot": "hat", "name": "Hat"}, 400)
	items := admin.call("GET", "/admin/api/cosmetics", nil, 200)["items"].([]any)
	if len(items) < 12 {
		t.Fatalf("expected seeded + new cosmetics, got %d", len(items))
	}
	rooms := admin.call("GET", "/admin/api/rooms", nil, 200)["rooms"].([]any)
	if len(rooms) < 2 {
		t.Fatalf("rooms: %d", len(rooms))
	}
	admin.call("POST", "/admin/api/rooms/"+g2["gameId"].(string)+"/end", nil, 204)
	if premium.call("GET", "/api/games/"+g2["gameId"].(string), nil, 200)["game"].(map[string]any)["status"] != "finished" {
		t.Fatal("room should be finished")
	}
	audit := admin.call("GET", "/admin/api/audit", nil, 200)["entries"].([]any)
	if len(audit) < 6 {
		t.Fatalf("audit entries: %d", len(audit))
	}
}
