# Monopsony

A web-based, real-time Monopoly-rules board game: Go backend, 3D React client, bots, free/premium tiers, paid cosmetics, and an admin panel for rules, names and skins.

See [PLAN.md](PLAN.md) for the architecture and phase plan.

## Layout

```
server/            Go module — rules engine, rooms, WebSocket, REST, billing, admin API
  cmd/server       the API/WS server (single binary)
  cmd/sim          headless bot-vs-bot simulator (engine fuzzing)
  internal/game    PURE rules engine (commands → events), Harbourside (shipped) + classic (test) boards, invariants
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
  scripts/brand.mjs  generates the logo, favicon, mascot and board emblem (public/brand)
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
go run ./cmd/sim -games 1000 -seed 42 -surrender 30    # ...with a random player surrendering in a random phase every ~30 commands
cd ..
npm run typecheck; npm run build
npm run e2e -w @monopsony/web                       # Playwright: boots a throwaway server on :8091 + Vite on :5199
```

The Playwright suite (`apps/web/e2e`) covers: guest → create table → bots → roll → buy/decline → end turn (with a board screenshot and the free-tier ad slot), reload mid-game restoring identical state, a cosmetic equipped by one player showing up in another player's client (including the glTF preview), and an admin publishing a config that renames Captain's Reach and changes start cash — the new game reflects it, the running one does not. `npm run e2e:ui -w @monopsony/web` opens the inspector. CI runs the Go suite against Postgres and Redis, the web build, and the e2e suite.

## Brand and default board

The shipped board is **Harbourside**, an original port-city theme (`server/internal/game/harbourside.go`): Kelp Lane to Captain's Reach, ferries instead of railroads, *Tide* and *Harbour Fund* decks, and *Set Sail / Dry Dock / Safe Harbour / Run Aground* corners. It keeps the classic layout space-for-space (a test enforces it) so the engine fixture (`ClassicConfig`, Hasbro's names, never shipped) still describes it. Card overlays and the admin editor take deck names from the board's own spaces, so a retheme needs no code change.

The logo is an emerald octagon coin with an "M" (eight sides: eight seats, eight arms); the mascot is **Mono**, an octopus in a captain's cap — one buyer, eight arms. `apps/web/scripts/brand.mjs` is the single source for all of it and writes the SVGs, PNG icons, the social card and the board-centre emblem (`npm run brand -w @monopsony/web`); `apps/web/src/brand` places them. Nothing borrows Monopoly's trade dress: no top hat, moustache, red banner wordmark or house/hotel iconography.

## Key design points

- **Server-authoritative, event-sourced gameplay.** The engine turns commands into events; the room persists a state snapshot plus the event log after every command. Clients animate the event stream (dice, token hops, cards) while receiving the authoritative state in an `Update` frame; reconnects replay events after `lastSeq`.
- **Bots are ordinary players.** They choose only from `LegalActions`, so they can never issue an illegal command. A human who times out gets the bot's safe default; after three timeouts a bot takes the seat. The host can do the same on demand: kicking a player mid-game hands their seat to a bot and detaches the user (`DELETE /api/games/{id}/seats/{playerId}`).
- **Anyone can surrender.** `Surrender` is legal in every phase, even mid-auction or while a trade is pending. It settles like a bankruptcy — everything to the creditor if the player owes one, otherwise back to the bank (deeds auctioned when the rules allow) — withdraws any live bid, and passes the turn on. `DeclareBankruptcy` remains the insolvent-only path.
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

The stack in `deploy/` is built for a box you own (a home server works) that already runs **Postgres** and a **Cloudflare Tunnel**: the compose adds only the app image and Redis. No port forwarding, Cloudflare terminates TLS, and the origin is reachable only through the tunnel.

```bash
cp deploy/.env.example deploy/.env      # then fill it in (secrets, public URL, admin email, Postgres creds)
docker compose -f deploy/docker-compose.prod.yml up -d --build
curl http://127.0.0.1:8080/healthz      # ok
```

One-time setup on the box:

1. **Database** — create the role and database in the existing Postgres container (`pg` is its container name, `$SUPERPW` the superuser password):

   ```bash
   docker exec -e PGPASSWORD=$SUPERPW pg psql -U postgres -v ON_ERROR_STOP=1 \
     -c "CREATE USER monopsony WITH PASSWORD 'a-long-random-password';" \
     -c "CREATE DATABASE monopsony OWNER monopsony;"
   ```

   Owning the database is all the app needs — no extensions, no extra grants; it applies its own migrations on boot. Put that password in `POSTGRES_PASSWORD`.

2. **Talking to Postgres** — by default the app connects to `host.docker.internal:5432`, the host gateway, which reaches whatever the Postgres container publishes on the host (`docker ps` shows `0.0.0.0:5432->5432/tcp`). That works no matter which network Postgres is on. Two alternatives:

   - **Container-to-container**, if you would rather not depend on the published port: find the network Postgres is attached to with `docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}' postgres`. If it names a *user-defined* network, add to `app` in the compose file

     ```yaml
     networks: [default, db]
     ```

     plus a top-level `networks: { db: { external: true, name: <that network> } }`, and set `POSTGRES_HOST` to the Postgres *container's* name. If it only says `bridge`, this will not work — Docker's default bridge has no container-name DNS — so stay with the host gateway.
   - **A Postgres that listens on the host itself** (not in Docker): same default, since `host.docker.internal` is the host either way.

3. **Tunnel** — in the tunnel already running on the box, add a public hostname (e.g. `beta.example.com`) → service `http://localhost:8080`. The compose binds the app to the host's loopback only, so nothing on the LAN reaches it without going through Cloudflare. WebSockets pass through by default. (If that `cloudflared` is a container rather than a host service, remove `ports` from `app`, attach it to the tunnel container's network too and use `http://app:8080`.)
4. **Country restriction** — your zone → Security → WAF → Custom rules: expression `(ip.src.country ne "MV")`, action *Block*. It is IP geolocation, so VPNs get around it in both directions. If you switch billing to Stripe, exempt the webhook: `(ip.src.country ne "MV" and not starts_with(http.request.uri.path, "/api/billing/webhook"))`.
5. **Closed beta (optional)** — Zero Trust → Access → Applications → *Self-hosted* on the same hostname, with a policy of Country = MV plus an email allowlist or one-time PIN. Free for up to 50 users and needs no app changes.
6. Security → Bots → turn *Bot Fight Mode* off (it interferes with API/WebSocket traffic).

Notes:

- `MONOPSONY_TRUST_PROXY` defaults to `1` in the prod compose because cloudflared is the only thing that can reach the app and it sets `X-Forwarded-For` to the visitor. Keep the port bound to `127.0.0.1` (never `0.0.0.0`), or LAN clients bypass both the country rule and the real-IP rate limits.
- `MONOPSONY_ENV=prod` (baked into the image) makes cookies `Secure`, which is why the stack only works behind HTTPS.
- `--scale app=N` runs several nodes; games are reachable from any node through Redis, so no sticky sessions. Uploaded assets live in the `media` volume; sync it to object storage if you run more than one node.
- Metrics are on `:9100` inside the compose network only.
- Backups: Postgres holds users and games; the `media` volume holds uploaded cosmetics. Something like this on a schedule:

  ```bash
  docker exec -e PGPASSWORD=$PW pg pg_dump -U monopsony monopsony | gzip > "backup-$(date +%Y%m%d).sql.gz"
  ```

- Going live with real money: `MONOPSONY_BILLING_PROVIDER=stripe`, the Stripe keys, and a premium `priceId` on the plan (admin → Plans).

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
