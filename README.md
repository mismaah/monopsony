# Monopsony

A web-based, real-time Monopoly-rules board game: Go backend, 3D React client, bots, free/premium tiers, paid cosmetics, and an admin panel for rules, names and skins.

See [PLAN.md](PLAN.md) for the architecture and phase plan.

## Layout

```
server/            Go module — rules engine, rooms, WebSocket, REST, billing, admin API
  cmd/server       the API/WS server (single binary)
  cmd/sim          headless bot-vs-bot simulator (engine fuzzing)
  internal/game    PURE rules engine (commands → events), classic config, invariants
  internal/bot     heuristic AI (also the "safe default" for timed-out humans)
  internal/room    one actor per live game: bots, timers, persistence, broadcast
  internal/lobby   room registry, entitlement checks, restore on boot
  internal/ws      WebSocket gateway
  internal/httpapi REST: auth, lobby, shop, billing webhooks
  internal/adminapi admin REST (configs, plans, users, cosmetics, rooms, audit)
  internal/store   Store interface + in-memory and Postgres implementations
  internal/auth    JWT sessions, argon2id passwords, guest accounts, Google/Discord OAuth
  internal/billing PaymentProvider interface: Stripe (REST + signed webhooks) and a dev fake
  internal/entitlement tier → capabilities (the only place free/premium rules live)
  internal/cosmetics catalog, ownership, loadouts, manifest validation, uploaded-asset store
  internal/cluster multi-node: Redis (or in-memory) bus, game ownership, cross-node room proxies
  internal/metrics Prometheus instruments;  internal/ratelimit token buckets
apps/web           player client: Vite + React + React Three Fiber + Tailwind (+ Playwright e2e)
apps/admin         admin panel: Vite + React + Tailwind
packages/protocol  TypeScript types generated from the Go structs (tygo) + command/event unions
packages/board-assets bundled glTF token models, the manifest schema, check/optimise/upload scripts
deploy/            docker-compose (dev Postgres+Redis, prod stack) and the production Dockerfile
```

## Run it locally

Prerequisites: Go 1.25+, Node 20+ (npm 10), Docker (for Postgres).

```powershell
npm install
docker compose -f deploy/docker-compose.yml up -d     # Postgres :5432 + Redis :6379 (optional; omit for in-memory)

# terminal 1 — API + WebSocket on :8080
cd server
$env:MONOPSONY_DATABASE_URL = "postgres://monopsony:monopsony@localhost:5432/monopsony?sslmode=disable"
$env:MONOPSONY_ADMIN_EMAILS = "you@example.com"
go run ./cmd/server

# terminal 2 — player app on http://localhost:5173 (proxies /api and /ws to :8080)
npm run dev -w @monopsony/web

# terminal 3 — admin panel on http://localhost:5174
npm run dev -w @monopsony/admin
```

Without `MONOPSONY_DATABASE_URL` the server uses an in-memory store (nothing survives a restart). `$env:` assignments last for the current terminal only. Copy `.env.example` to `.env` (`Copy-Item .env.example .env`) for the full list of settings; `task server` loads it automatically if you use [Task](https://taskfile.dev), otherwise `go run` ignores `.env`.

Play: open the player app → *Play as guest* → *Create table* → add bots → *Start game*. Register with the email in `MONOPSONY_ADMIN_EMAILS` to get into the admin panel.

## Tests

```powershell
cd server; go test -race ./...                         # engine, room, store (mem), billing, cluster (mem bus), HTTP+WS end-to-end
$env:MONOPSONY_TEST_DATABASE_URL = "postgres://..."; go test ./internal/store   # also runs the Postgres conformance suite
$env:MONOPSONY_TEST_REDIS_URL = "redis://localhost:6379/1"; go test ./internal/cluster  # also runs the Redis bus + RPC test
go run ./cmd/sim -games 1000 -seed 42 -players 4       # fuzz the engine: invariants checked after every command
cd ..
npm run typecheck; npm run build
npm run e2e -w @monopsony/web                       # Playwright: boots a throwaway server on :8091 + Vite on :5199
```

The Playwright suite (`apps/web/e2e`) covers: guest → create table → bots → roll → buy/decline → end turn (with a board screenshot and the free-tier ad slot), reload mid-game restoring identical state, a cosmetic equipped by one player showing up in another player's client (including the glTF preview), and an admin publishing a config that renames Boardwalk and changes start cash — the new game reflects it, the running one does not. `npm run e2e:ui -w @monopsony/web` opens the inspector. CI runs the Go suite against Postgres and Redis, the web build, and the e2e suite.

## Key design points

- **Server-authoritative, event-sourced gameplay.** The engine turns commands into events; the room persists a state snapshot plus the event log after every command. Clients animate the event stream (dice, token hops, cards) while receiving the authoritative state in an `Update` frame; reconnects replay events after `lastSeq`.
- **Bots are ordinary players.** They choose only from `LegalActions`, so they can never issue an illegal command. A human who times out gets the bot's safe default; after three timeouts a bot takes the seat.
- **Entitlements in one place.** `entitlement.Resolve` maps a tier to capabilities (max players, private rooms, house rules, ads, stats window, cosmetic slots). Admins edit plans live; the lobby and shop enforce them server-side.
- **Versioned configs.** Admins publish immutable config versions (board names/prices, card decks, rule defaults). New tables use the published version; running games keep theirs.
- **Cosmetics are data.** Items carry a manifest the client renders: a builtin shape, a palette (boards/dice/cards) or a glTF model — bundled with the client (`{"model":{"builtin":"tophat"}}`) or uploaded through the admin panel and served under `/media/` (content-addressed, cached forever). Manifests are validated server-side (`internal/cosmetics/manifest.go`; JSON Schema in `packages/board-assets/manifest.schema.json`), the client fetches them once (`GET /api/cosmetics/manifests`), and loadouts are broadcast with seats so opponents see your skin. Materials named `keep_*` in a model keep their authored colour; everything else is tinted with the seat colour.
- **Multi-node.** Set `MONOPSONY_REDIS_URL` on every server (all sharing one Postgres) and they form a cluster: each game is owned by one node (its room actor runs there), other nodes proxy commands over Redis pub/sub and relay the room's frames to their own WebSocket clients, public listings come from the store, invite codes resolve cluster-wide. Ownership is a Redis key guarded by a node liveness heartbeat; when a node dies (or drains on shutdown) its games are adopted from the store by the next node that needs them, and affected clients are told to rejoin. `internal/cluster` has an in-memory bus for tests.
- **Hardening.** Per-IP token-bucket rate limits by route family (auth / API / WebSocket upgrade) plus per-connection frame and chat budgets; Prometheus metrics on a separate listener (`:9100/metrics`: HTTP by route/status, WS connections and frames, commands/events/bot actions, rooms per status, rate-limit rejections, cluster RPCs); JSON logging; shutdown drains rooms and leaves the cluster.
- **Ads.** `<AdSlot/>` renders only for `showAds` users. The provider comes from `GET /api/config`: `house` (built-in upgrade promo, the default), `adsense` (Google AdSense, `MONOPSONY_ADSENSE_CLIENT` + `_SLOT`, optional non-personalised mode) or `none`.

## Environment

| Variable | Purpose |
| --- | --- |
| `MONOPSONY_ADDR` | listen address (default `:8080`) |
| `MONOPSONY_DATABASE_URL` | Postgres DSN; unset = in-memory store |
| `MONOPSONY_JWT_SECRET` | HS256 secret for access tokens |
| `MONOPSONY_PUBLIC_URL` | browser-facing origin (billing redirects, OAuth callbacks) |
| `MONOPSONY_CORS_ORIGINS` | comma-separated dev origins |
| `MONOPSONY_ADMIN_EMAILS` | emails that receive the admin role |
| `MONOPSONY_BILLING_PROVIDER` | `fake` (default, instant fulfilment) or `stripe` |
| `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` | Stripe credentials; webhook endpoint is `POST /api/billing/webhook` |
| `GOOGLE_CLIENT_ID/SECRET`, `DISCORD_CLIENT_ID/SECRET` | enable OAuth sign-in |
| `MONOPSONY_STATIC_DIR` | serve built SPAs (player at `/`, admin at `/admin/`) |
| `MONOPSONY_BOT_DELAY_MS` | pause before bots act (animation pacing) |
| `MONOPSONY_LOG_LEVEL`, `MONOPSONY_LOG_FORMAT` | `debug\|info\|warn\|error`, `text\|json` |
| `MONOPSONY_METRICS_ADDR` | Prometheus listener (default `:9100`, `off` to disable) |
| `MONOPSONY_RATE_AUTH/API/WS/WS_FRAMES/WS_CHAT` | budgets as `per-minute/burst`; `0` disables one |
| `MONOPSONY_TRUST_PROXY` | `1` to honour `X-Forwarded-For` (behind your own proxy only) |
| `MONOPSONY_REDIS_URL`, `MONOPSONY_NODE_ID` | join a multi-node cluster (needs shared Postgres) |
| `MONOPSONY_ASSET_DIR` | uploaded cosmetic assets (default `./data/assets`, `off` disables uploads) |
| `MONOPSONY_ADS_PROVIDER`, `MONOPSONY_ADSENSE_CLIENT/SLOT`, `MONOPSONY_ADS_NON_PERSONALIZED` | free-tier ads: `house` (default), `adsense`, `none` |

## Production

`docker compose -f deploy/docker-compose.prod.yml up --build` builds a single image (Go server + both SPAs) next to Postgres and Redis. Set `MONOPSONY_JWT_SECRET`, `MONOPSONY_PUBLIC_URL`, `POSTGRES_PASSWORD` and, when going live, `MONOPSONY_BILLING_PROVIDER=stripe` with the Stripe keys and a premium `priceId` on the plan (admin → Plans). `--scale app=N` runs several nodes (each gets a host port from `8080-8089`; put your load balancer in front — no sticky sessions needed). Scrape `:9100/metrics` from inside the network; keep `MONOPSONY_TRUST_PROXY=1` only when the balancer sets `X-Forwarded-For`. Uploaded assets live in the `media` volume; sync it to object storage/CDN if you run more than one node.

## Cosmetic assets

```powershell
npm run build -w @monopsony/board-assets          # regenerate the bundled models (models/*.glb, committed)
npm run check -w @monopsony/board-assets -- my.glb   # inspect a model before uploading: size, tris, normals, bounds
npm run optimize -w @monopsony/board-assets -- my.glb out.glb --compress draco   # via @gltf-transform/cli (npx)
$env:MONOPSONY_ADMIN_EMAIL = "..."; $env:MONOPSONY_ADMIN_PASSWORD = "..."; npm run upload -w @monopsony/board-assets -- out.glb
```

`upload` prints the `/media/<hash>.glb` URL and a manifest snippet; paste it into admin → Cosmetics (which also offers upload, templates per slot and a live *Validate*). Fit a model with `model.scale/offset/rotation` — the token footprint is ~0.2 units radius, 0.5 tall, base at the origin.

## Notes

- "Monopoly", the classic street names and card texts are Hasbro trademarks/copyright. The bundled *Classic* config exists as a development fixture; use the admin panel to publish an original theme before shipping.
- Trades, auctions, mortgages, even-build, bankruptcy and the house-shortage rules follow the standard rulebook; house rules (Free Parking jackpot, no auctions, double Go salary, no rent in jail, turn limit) are premium-gated toggles.
