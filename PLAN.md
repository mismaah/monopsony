# Monopsony — Web-based Monopoly: Architecture & Implementation Plan

## Context

Empty repo. Goal: an online, real-time Monopoly-rules board game with a Go backend, a 3D/animated web client, a free/paid tier split, paid cosmetics, and an admin panel that can customize rules, names, and skins. Toolchain present on this machine: Go 1.25, Node 20 + npm, Docker 29.

Decisions already made with the user:
- **3D frontend:** React + React Three Fiber (Three.js).
- **Multiplayer:** online real-time, up to 8 seats, bots fill empty seats.
- **Free-tier limits:** fewer players / no private rooms; standard rules only (no house rules); ads + default cosmetics only + limited stats history.
- **Auth/payments:** pluggable interfaces; ship a concrete Stripe + local (email/password + OAuth) implementation first.

Judgment calls made here (change if you disagree):
- Monorepo (npm workspaces + one Go module), single Go binary serving REST + WebSocket + admin API.
- Postgres for persistence; no Redis in v1 (rooms are in-memory on one node, event log persisted for recovery). Redis pub/sub is the documented path to multi-node later.
- JSON over WebSocket (not protobuf) — easier to debug; TS types are generated from Go structs so both sides stay in sync.
- Dice/token animation is **server-decided, client-animated** (no client physics deciding outcomes).
- Trademark note: "Monopoly", the board names, and the card texts are Hasbro IP. The admin config system makes every name/price/card editable so the shipped default set can be an original theme; the classic layout is fine as a dev/test fixture.

---

## 1. Repository layout

```
Monopsony/
├─ server/                     # Go module: github.com/<you>/monopsony/server
│  ├─ cmd/server/main.go       # single binary: API + WS + admin API + static hosting (optional)
│  ├─ cmd/sim/main.go          # headless bot-vs-bot simulator for rules-engine testing
│  ├─ internal/
│  │  ├─ game/                 # PURE rules engine: state, commands, events, reducer. No I/O.
│  │  ├─ room/                 # one actor (goroutine) per live game; timers; bots; broadcast
│  │  ├─ lobby/                # room creation, public matchmaking, invite codes
│  │  ├─ ws/                   # WebSocket gateway, per-connection session, envelope codec
│  │  ├─ protocol/             # Go structs for every WS message (source of truth for TS types)
│  │  ├─ auth/                 # Provider interface + local + OAuth impls; JWT access/refresh
│  │  ├─ billing/              # PaymentProvider interface + Stripe impl + webhook handler
│  │  ├─ entitlement/          # user -> Tier + owned cosmetics -> Capabilities (single gate)
│  │  ├─ config/               # versioned GameConfig (board, decks, rule toggles); publish flow
│  │  ├─ cosmetics/            # catalog, asset manifests, loadouts
│  │  ├─ bot/                  # AI player strategy (heuristic)
│  │  ├─ store/                # pgx + sqlc queries, migrations (goose)
│  │  ├─ httpapi/              # REST handlers (std net/http ServeMux, Go 1.22+ patterns)
│  │  └─ adminapi/             # admin REST handlers, role-gated
│  └─ migrations/
├─ apps/
│  ├─ web/                     # player client: Vite + React 19 + TS + R3F + drei + Zustand + Tailwind
│  └─ admin/                   # admin panel: Vite + React + TS + Tailwind (no 3D except skin preview)
├─ packages/
│  ├─ protocol/                # TS types GENERATED from server/internal/protocol via tygo
│  ├─ ui/                      # shared React components (buttons, modals, auth forms)
│  └─ board-assets/            # glTF/textures + manifest schema; default skin set
├─ deploy/                     # Dockerfiles, docker-compose.yml (postgres + server + web)
├─ Taskfile.yml                # dev tasks (gen, lint, test, sim, up)
└─ README.md
```

Key third-party choices (Go): `github.com/coder/websocket`, `github.com/jackc/pgx/v5`, `sqlc`, `pressly/goose`, `golang-jwt/jwt/v5`, `stripe/stripe-go`, `gtramontina/tygo` (Go→TS types). Frontend: `three`, `@react-three/fiber`, `@react-three/drei`, `@react-three/postprocessing`, `zustand`, `gsap` (timelines for token/dice/camera), `framer-motion` (2D HUD), `react-router`, `tailwindcss`.

---

## 2. Backend design (Go)

### 2.1 Rules engine — `internal/game` (pure, deterministic)

```go
type State struct { Config ConfigRef; Players []Player; Board []SpaceState; Turn TurnState; Bank Bank; Decks Decks; RNGSeed …; Phase Phase; Pending *Pending }
type Command interface{ PlayerID() string }     // RollDice, BuyProperty, DeclineBuy, PlaceBid, PassBid, EndTurn,
                                                // BuildHouse, SellHouse, Mortgage, Unmortgage, ProposeTrade,
                                                // AcceptTrade, RejectTrade, PayJailFine, UseJailCard, DeclareBankruptcy
type Event interface{ EventType() string }      // DiceRolled, TokenMoved, PassedGo, PropertyBought, AuctionStarted,
                                                // BidPlaced, AuctionEnded, RentPaid, TaxPaid, CardDrawn, CardResolved,
                                                // SentToJail, LeftJail, HouseBuilt, HouseSold, Mortgaged, Unmortgaged,
                                                // TradeProposed, TradeResolved, DebtOwed, PlayerBankrupt, TurnChanged, GameEnded
func Apply(s *State, cmd Command, rng RNG) ([]Event, error)   // validates, mutates, returns events
func Replay(cfg Config, events []Event) *State                // rebuild state from log
func LegalActions(s *State, playerID string) []ActionDescriptor // drives UI buttons + bots
```

Phases (per turn): `PreRoll` → `Rolling` → `Moving` → `Resolving` (buy/auction/rent/card/tax) → `PostRoll` → doubles loop back to `Rolling` or `EndTurn`. Side states: `Auction`, `TradePending`, `RaisingFunds` (debt), `Jail`.

Standard rules to implement (all covered by table-driven tests):
- 40 spaces: 22 streets / 8 color groups, 4 railroads, 2 utilities, Go, Jail, Free Parking, Go To Jail, 3 Chance, 3 Community Chest, Income Tax 200, Luxury Tax 100. Start cash 1500; Go salary 200.
- Doubles roll again; third consecutive doubles → jail. Jail: pay 50, use card, or roll doubles (3 tries then must pay).
- Landing on unowned property: buy at list price or **auction** (all players, any starting bid).
- Rent: base; ×2 for unimproved monopoly; house/hotel table; railroads 25/50/100/200; utilities ×4 / ×10 (×10 on card-directed moves).
- Building: must own full color group, **even-build rule**, bank supply 32 houses / 12 hotels (shortage → deny build in v1; building-shortage auction is a v2 toggle). Selling buildings at half price, even-sell rule.
- Mortgage: half price; unmortgage at +10%; must sell buildings first; mortgaged properties collect no rent (group still counts for doubling on other unmortgaged streets).
- Trades: properties, cash, GOOJF cards between any two players; mortgaged property transfer charges 10% immediately or on unmortgage.
- Debt: on insufficient cash player enters `RaisingFunds`; must sell/mortgage until covered or `DeclareBankruptcy`. Bankruptcy to a player transfers everything; to the bank → properties auctioned.
- Cards: 16 Chance + 16 Community Chest (texts/effects come from config; classic set as fixture). GOOJF is held and removed from the deck until used.
- Win: last player not bankrupt. Optional turn-limit/time-limit ends → net worth wins (house rule toggle).
- Turn timer: configurable (default 60 s for decisions, 30 s auction bids); on timeout the server auto-picks the safe action (decline buy, pass bid, end turn) and after N timeouts swaps the player for a bot.

House rules (paid tier, config toggles): Free Parking jackpot, no auctions, double salary for landing on Go, no rent in jail, snake eyes bonus, turn/time limit.

`cmd/sim` runs thousands of bot-vs-bot games with a fixed seed to catch invariants (cash conservation, house supply, never negative balances, game always terminates).

### 2.2 Live games — `internal/room`

- One `Room` goroutine per game owns a `*game.State`; inbox channel of `(playerID, Command)`; every `Apply` result is (a) appended to `game_events` in Postgres, (b) fanned out to connected clients, (c) fed to bots.
- Bots run as `Player` implementations that receive events and reply with commands via the same inbox (so they're indistinguishable to the engine).
- Reconnect: client sends `Resume{gameID, lastSeq}`; room replies with `StateSnapshot` + events since `lastSeq`.
- Crash recovery: on boot, rooms with status `in_progress` are rebuilt via `Replay` from the event log.
- Spectator mode is free (read-only fan-out).

### 2.3 WebSocket protocol — `internal/protocol`

Envelope `{ "t": "<Type>", "seq": n, "p": {...} }`. Client→server = commands + `Resume`/`Ping`. Server→client = `StateSnapshot`, every game `Event`, `LegalActions`, `Error`, `LobbyUpdate`. `tygo` generates `packages/protocol/src/generated.ts` from the Go structs; `task gen` is CI-enforced (fails if out of date).

### 2.4 Auth — `internal/auth`

```go
type Provider interface { Begin(ctx, redirect) (url string); Complete(ctx, callback) (Identity, error) }
```
Implementations: `local` (email + argon2id password), `google`, `discord` (OAuth2). Session = short-lived JWT access token (15 min) + httpOnly refresh cookie stored in `refresh_tokens`. Roles: `player`, `admin`. WS auth = access token in the first frame.

### 2.5 Billing — `internal/billing`

```go
type PaymentProvider interface {
  CreateCheckout(ctx, user, sku SKU) (url string, err error)     // subscription or one-off cosmetic
  CustomerPortal(ctx, user) (url string, err error)
  HandleWebhook(ctx, payload, sig) ([]BillingEvent, error)       // SubscriptionActivated/Canceled, PurchaseCompleted, Refunded
}
```
Stripe impl uses Checkout Sessions + Customer Portal; webhooks update `subscriptions` / `purchases` idempotently (store `event_id`). A `fake` provider is used in dev/tests to grant/revoke instantly.

### 2.6 Entitlements — `internal/entitlement` (the only place tier rules live)

```go
type Capabilities struct {
  MaxPlayersPerRoom int   // free 4, paid 8
  PrivateRooms      bool  // free false
  HouseRules        bool  // free false
  ShowAds           bool  // free true
  StatsHistoryDays  int   // free 7, paid unlimited (0 = unlimited)
  CosmeticsAllowed  []CosmeticID // defaults + owned
}
func Resolve(ctx, userID) (Capabilities, error)
```
Lobby, room, config, and cosmetics endpoints call `Resolve` and enforce server-side; the client only uses it to grey out UI. Tier numbers are stored in `plans` table so admins can edit them without a deploy.

### 2.7 Game config — `internal/config`

`GameConfig` JSON: `{ board: [40 spaces], chanceDeck, communityDeck, rules: {startCash, goSalary, jailFine, houseSupply, hotelSupply, auctionsEnabled, freeParkingJackpot, …}, theme: {currencySymbol, names…} }`. Rows in `game_configs` are immutable versions; `published_config_id` pointer chooses the default for new games. A game pins its `config_id` for life so admin edits never break running games. Validation on save (40 spaces, 8 groups with correct sizes, monotonic rents, deck non-empty, etc.).

### 2.8 Cosmetics — `internal/cosmetics`

Types: `token`, `board_skin`, `dice`, `buildings`, `card_back`, `emote`. Each has an **asset manifest** (glTF URLs, texture URLs, material params, preview image) served from `/assets/…` (local disk in dev, S3/CDN-compatible path in prod). `user_cosmetics` = ownership; `user_loadout` = equipped set. Room broadcasts each player's loadout in `StateSnapshot` so opponents render your skin.

### 2.9 Bots — `internal/bot`

Heuristic v1: buy if cash after purchase ≥ reserve; bid up to ~1.2× value if it completes/blocks a monopoly; build evenly when monopoly owned and cash ≥ reserve; accept trades with positive net-worth delta (simple valuation table); pay jail fine early game, sit late game. Difficulty = reserve/aggression parameters. Interface lets a smarter bot drop in later.

### 2.10 Data model (Postgres)

`users`, `auth_identities`, `refresh_tokens`, `plans`, `subscriptions`, `purchases`, `billing_webhook_events`, `cosmetics`, `user_cosmetics`, `user_loadouts`, `game_configs`, `games`, `game_players`, `game_events` (append-only: game_id, seq, type, payload jsonb), `game_results` (stats), `admin_audit_log`.

---

## 3. Player frontend — `apps/web`

- **Stack:** Vite + React 19 + TS, R3F + drei, GSAP timelines for 3D motion, Framer Motion for HUD, Zustand store, react-router, Tailwind.
- **Scene:** `<Board/>` (glTF, skin-swappable material set), `<Token/>` ×N, `<Dice/>`, `<Buildings/>` (instanced houses/hotels), `<CameraRig/>` (orbit + scripted moves: follow token, zoom on property, overview), `<Effects/>` (bloom/SSAO, toggleable for low-end).
- **Event → animation pipeline:** WS events are pushed onto an `AnimationQueue`; each event maps to a GSAP timeline (dice tumble that lands on the server's faces, token hop-by-space along a spline, cash counter tick, card flip, house pop-in). State store updates when the timeline completes (or immediately with a "settling" flag for fast-forward/reconnect). Spectators and late-joiners get the snapshot with no animation.
- **HUD:** turn banner, action bar driven by `LegalActions`, property panel (build/mortgage/sell), trade builder, auction modal, chat/emotes, player rail with cash + owned groups, timer ring. Ad slot component (`<AdSlot/>`, renders nothing when `ShowAds=false`; provider TBD).
- **Routes:** `/` landing, `/login`, `/lobby` (public rooms, create room, invite code), `/game/:id`, `/shop`, `/profile` (stats, loadout), `/account` (plan, portal link).
- **Performance:** glTF Draco-compressed, instanced meshes for buildings, `<AdaptiveDpr/>`, quality preset in settings.

## 4. Admin frontend — `apps/admin`

Separate Vite app, admin-role login, talks to `/admin/api/*`.
- **Board & rules editor:** table editor for the 40 spaces (name, group, color, price, rents, mortgage), card deck editor (text + effect type + params), rule toggles and defaults, theme (currency symbol, game name). Draft → validate → publish as new config version; diff view between versions.
- **Cosmetics catalog:** create/edit items, upload assets (manifest + files), price, tier requirement, enable/disable; 3D preview using the same R3F components from `packages/ui`.
- **Plans & tiers:** edit `plans` rows (limits, price IDs).
- **Users:** search, view subscription/purchases, grant/revoke tier or cosmetic, ban.
- **Live ops:** list active rooms, spectate, force-end, view event log.
- Every mutation lands in `admin_audit_log`.

---

## 5. Implementation phases

Each phase ends green (lint + tests + `task up` works).

**Phase 0 — Scaffold (1 session)**
Init git, npm workspaces, Go module, Taskfile, docker-compose (Postgres), goose migrations skeleton, CI (GitHub Actions: `go vet`/`golangci-lint`/`go test`, `npm run lint`/`typecheck`), README with run instructions, `.editorconfig`.

**Phase 1 — Rules engine**
`internal/game` complete with classic config fixture; table-driven tests per rule area (movement, rent, jail, auctions, building, mortgage, trades, debt/bankruptcy, cards); `cmd/sim` invariant fuzzing with random bots. This phase is the foundation and should be the most heavily tested code in the repo.

**Phase 2 — Server core**
`store` + migrations, `auth` (local first), `protocol` + `tygo` gen, `ws` gateway, `room` actor with persistence/replay/reconnect, `lobby`, REST for auth/lobby. A throwaway 2D debug client (plain HTML table board) in `apps/web` to exercise the flow end-to-end before 3D work.

**Phase 3 — 3D client**
Board/tokens/dice/buildings scene, camera rig, animation queue, HUD + action bar from `LegalActions`, trade/auction UIs, reconnect UX. Default skin set in `packages/board-assets`.

**Phase 4 — Bots**
`internal/bot` heuristic, fill-with-bots in lobby, bot takeover on abandon, difficulty setting.

**Phase 5 — Monetization**
`entitlement`, `plans` table, `billing` interface + fake + Stripe (Checkout, Portal, webhooks), shop + account pages, cosmetics catalog/loadout/manifest loading in the 3D scene, `<AdSlot/>`, stats history window.

**Phase 6 — Admin**
`adminapi`, `apps/admin` (config editor + publish, cosmetics, plans, users, live ops), audit log, OAuth providers (Google/Discord).

**Phase 7 — Hardening & deploy**
Rate limiting, structured logging + metrics (OpenTelemetry/Prometheus), graceful shutdown with room drain, Dockerfiles, prod compose/Fly/Render config, load test with `cmd/sim` bots over real WS.

---

## 6. Verification

- **Engine:** `go test ./internal/game/...` — table-driven tests for every rule above; `go run ./cmd/sim -games 10000 -seed 42` must finish with zero invariant violations (cash conserved across bank+players, supply counts, termination).
- **Server:** integration tests spin up Postgres via `testcontainers-go`; a test creates a room, connects 3 WS clients + 1 bot, plays scripted commands, kills a client, reconnects with `lastSeq`, asserts identical state; restart the server mid-game and assert `Replay` rebuilds the room.
- **Protocol sync:** CI runs `task gen` and fails on git diff.
- **Entitlements:** unit tests for `Resolve` per plan; HTTP tests assert a free user is rejected creating a private room / 8-seat room / house-rule game, and accepted after the fake billing provider activates a subscription; Stripe webhook handler tested with recorded fixtures and duplicate-delivery idempotency.
- **Frontend:** Vitest for store/animation-queue reducers; Playwright smoke: login → create room → add bots → roll → buy → end turn, screenshot of the 3D canvas; manual check of reconnect (refresh mid-game) and of equipping a cosmetic showing on another client.
- **Admin:** publish a config that renames Boardwalk and changes start cash → new game reflects it, in-progress game does not.
- **End-to-end local run:** `task up` (compose: postgres + server + web + admin) then play a 4-player game with 3 bots in the browser.

---

## Status (2026-09-18)

Implemented and verified in this repo:

- **Phase 0–1** — monorepo scaffold, Go rules engine (`server/internal/game`) with 24 table-driven tests, invariant checker, `cmd/sim` fuzzing (0 failures across thousands of 2–8 player games), tygo-generated TS types.
- **Phase 2** — store (in-memory + Postgres with embedded migrations, conformance-tested against both), room actor (bots, turn timers, bot takeover, snapshot/event persistence, reconnect replay, restart recovery), lobby manager with entitlement checks, WebSocket gateway, REST auth/lobby API, single-binary server. HTTP+WS end-to-end tests.
- **Phase 3** — React Three Fiber client: procedural 3D board, animated tokens/dice/buildings, event-driven animation queue, HUD (players, action bar for every phase incl. auctions/jail/debt, deed panel, trade builder, card overlay, log/chat, game-over). Verified in headless Chromium.
- **Phase 4** — heuristic bots with three profiles, trade proposals, safe defaults for timeouts.
- **Phase 5** — entitlements (admin-editable plans), billing (`PaymentProvider` with Stripe REST + signed webhooks and a dev fake), cosmetics catalog/ownership/loadouts broadcast to the room, shop + account pages, ad slot.
- **Phase 6** — admin API + admin app: versioned config editor (rules/spaces/cards/JSON) with validate + publish, plans, users (tier/role/ban/grant), cosmetics, live rooms (force end), audit log. Google/Discord OAuth providers.
- **Phase 7** — rate limiting (per-IP by route family + per-connection WebSocket budgets), Prometheus metrics on a separate listener, JSON logging, graceful shutdown with room drain, Dockerfile + prod compose (Postgres + Redis, scalable app), GitHub Actions CI (Go suite against Postgres and Redis, web build, Playwright e2e).
- **Multi-node** — `internal/cluster`: Redis pub/sub + keys (in-memory bus for tests); one owner node per game, cross-node room proxies with RPC and frame relay, cluster-wide listings/invite codes, liveness heartbeats, adoption of orphaned games from the store, clients told to rejoin. Verified with two in-process nodes and against a real Redis.
- **glTF asset pipeline** — `packages/board-assets` (dependency-free GLB writer, two bundled token models generated procedurally and committed, `check` / `optimize` / `upload` scripts, manifest JSON Schema); server-side manifest validation, admin asset store (content-addressed, served under `/media/` with immutable caching), admin UI for uploads/templates/validation; client resolves loadouts through manifests and renders glTF tokens (bundled or uploaded, seat-tinted with `keep_*` materials preserved) with builtin-shape fallback; board/dice palettes are manifest-driven too.
- **Ads** — `<AdSlot/>` with a server-selected provider (`house` promo, Google AdSense, or none) in the game, lobby and shop.
- **Playwright** — `apps/web/e2e`: smoke play-through with screenshot, reconnect, cross-client cosmetics, admin config publish semantics; runs in CI on a throwaway server.

Not yet done: nothing from this plan. Candidates for a v2: an S3/CDN-backed asset store (the local store's URL space is designed for it), OpenTelemetry tracing, a building-shortage auction toggle, and load-testing the cluster with `cmd/sim` bots over real WebSockets.
