package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"monopsony/server/internal/game"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// PG is the Postgres Store.
type PG struct {
	pool *pgxpool.Pool
}

// OpenPG connects and applies pending migrations.
func OpenPG(ctx context.Context, url string) (*PG, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	p := &PG{pool: pool}
	if err := p.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

func (p *PG) migrate(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := p.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func nullEmail(e string) any {
	if e == "" {
		return nil
	}
	return strings.ToLower(e)
}

// ---- users -----------------------------------------------------------------------

const userCols = `id, COALESCE(email,''), name, password_hash, role, tier, guest, banned, created_at`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.Tier, &u.Guest, &u.Banned, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (p *PG) CreateUser(ctx context.Context, u *User) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO users(id,email,name,password_hash,role,tier,guest,banned,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		u.ID, nullEmail(u.Email), u.Name, u.PasswordHash, u.Role, u.Tier, u.Guest, u.Banned, u.CreatedAt)
	if isUnique(err) {
		return ErrConflict
	}
	return err
}

func (p *PG) GetUser(ctx context.Context, id string) (*User, error) {
	return scanUser(p.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
}

func (p *PG) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(p.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE email=$1`, strings.ToLower(email)))
}

func (p *PG) UpdateUser(ctx context.Context, u *User) error {
	tag, err := p.pool.Exec(ctx, `UPDATE users SET email=$2,name=$3,password_hash=$4,role=$5,tier=$6,guest=$7,banned=$8 WHERE id=$1`,
		u.ID, nullEmail(u.Email), u.Name, u.PasswordHash, u.Role, u.Tier, u.Guest, u.Banned)
	if err != nil {
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- sessions ---------------------------------------------------------------------

func (p *PG) PutRefreshToken(ctx context.Context, t RefreshToken) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO refresh_tokens(hash,user_id,expires_at) VALUES($1,$2,$3) ON CONFLICT (hash) DO UPDATE SET expires_at=EXCLUDED.expires_at`, t.Hash, t.UserID, t.ExpiresAt)
	return err
}

func (p *PG) GetRefreshToken(ctx context.Context, hash string) (*RefreshToken, error) {
	var t RefreshToken
	err := p.pool.QueryRow(ctx, `SELECT hash,user_id,expires_at FROM refresh_tokens WHERE hash=$1 AND expires_at > now()`, hash).Scan(&t.Hash, &t.UserID, &t.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &t, err
}

func (p *PG) DeleteRefreshToken(ctx context.Context, hash string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE hash=$1`, hash)
	return err
}

// ---- games -------------------------------------------------------------------------

const gameCols = `id,name,status,host_id,visibility,COALESCE(invite_code,''),max_players,turn_seconds,config_id,config,seats,state,seq,COALESCE(winner_id,''),created_at,updated_at,started_at,finished_at`

func scanGame(row pgx.Row) (*GameRecord, error) {
	var g GameRecord
	var cfg, seats []byte
	var state []byte
	if err := row.Scan(&g.ID, &g.Name, &g.Status, &g.HostID, &g.Visibility, &g.InviteCode, &g.MaxPlayers, &g.TurnSeconds,
		&g.ConfigID, &cfg, &seats, &state, &g.Seq, &g.WinnerID, &g.CreatedAt, &g.UpdatedAt, &g.StartedAt, &g.FinishedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(cfg, &g.Config); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(seats, &g.Seats); err != nil {
		return nil, err
	}
	if len(state) > 0 {
		g.State = json.RawMessage(state)
	}
	return &g, nil
}

func (p *PG) SaveGame(ctx context.Context, g *GameRecord) error {
	cfg, err := json.Marshal(g.Config)
	if err != nil {
		return err
	}
	seats, err := json.Marshal(g.Seats)
	if err != nil {
		return err
	}
	var state any
	if len(g.State) > 0 {
		state = []byte(g.State)
	}
	var invite any
	if g.InviteCode != "" {
		invite = g.InviteCode
	}
	var winner any
	if g.WinnerID != "" {
		winner = g.WinnerID
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO games(id,name,status,host_id,visibility,invite_code,max_players,turn_seconds,config_id,config,seats,state,seq,winner_id,created_at,updated_at,started_at,finished_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name,status=EXCLUDED.status,host_id=EXCLUDED.host_id,visibility=EXCLUDED.visibility,
		invite_code=EXCLUDED.invite_code,max_players=EXCLUDED.max_players,turn_seconds=EXCLUDED.turn_seconds,seats=EXCLUDED.seats,state=EXCLUDED.state,
		seq=EXCLUDED.seq,winner_id=EXCLUDED.winner_id,updated_at=EXCLUDED.updated_at,started_at=EXCLUDED.started_at,finished_at=EXCLUDED.finished_at`,
		g.ID, g.Name, g.Status, g.HostID, g.Visibility, invite, g.MaxPlayers, g.TurnSeconds, g.ConfigID, cfg, seats, state, g.Seq, winner,
		g.CreatedAt, g.UpdatedAt, g.StartedAt, g.FinishedAt)
	return err
}

func (p *PG) GetGame(ctx context.Context, id string) (*GameRecord, error) {
	return scanGame(p.pool.QueryRow(ctx, `SELECT `+gameCols+` FROM games WHERE id=$1`, id))
}

func (p *PG) queryGames(ctx context.Context, q string, args ...any) ([]*GameRecord, error) {
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*GameRecord
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (p *PG) ListGames(ctx context.Context, status string, publicOnly bool, limit int) ([]*GameRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT ` + gameCols + ` FROM games WHERE ($1='' OR status=$1) AND (NOT $2 OR visibility='public') ORDER BY created_at DESC LIMIT $3`
	return p.queryGames(ctx, q, status, publicOnly, limit)
}

func (p *PG) ListGamesForUser(ctx context.Context, userID string, limit int) ([]*GameRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	needle, _ := json.Marshal([]map[string]string{{"userId": userID}})
	q := `SELECT ` + gameCols + ` FROM games WHERE seats @> $1::jsonb ORDER BY updated_at DESC LIMIT $2`
	return p.queryGames(ctx, q, string(needle), limit)
}

func (p *PG) AppendEvents(ctx context.Context, gameID string, events []game.EventEnvelope) error {
	if len(events) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range events {
		batch.Queue(`INSERT INTO game_events(game_id,seq,type,payload) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, gameID, e.Seq, e.Type, []byte(e.Payload))
	}
	br := p.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range events {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (p *PG) GetEvents(ctx context.Context, gameID string, afterSeq int) ([]game.EventEnvelope, error) {
	rows, err := p.pool.Query(ctx, `SELECT seq,type,payload FROM game_events WHERE game_id=$1 AND seq>$2 ORDER BY seq`, gameID, afterSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []game.EventEnvelope
	for rows.Next() {
		var e game.EventEnvelope
		var payload []byte
		if err := rows.Scan(&e.Seq, &e.Type, &payload); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- configs ------------------------------------------------------------------------

const configCols = `id,version,name,config,published,created_by,created_at`

func scanConfig(row pgx.Row) (*ConfigRecord, error) {
	var c ConfigRecord
	var raw []byte
	if err := row.Scan(&c.ID, &c.Version, &c.Name, &raw, &c.Published, &c.CreatedBy, &c.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, &c.Config); err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *PG) SaveConfig(ctx context.Context, c *ConfigRecord) error {
	raw, err := json.Marshal(c.Config)
	if err != nil {
		return err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if c.Published {
		if _, err := tx.Exec(ctx, `UPDATE game_configs SET published=FALSE WHERE published`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO game_configs(id,version,name,config,published,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.Version, c.Name, raw, c.Published, c.CreatedBy, c.CreatedAt); err != nil {
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	return tx.Commit(ctx)
}

func (p *PG) GetConfig(ctx context.Context, id string) (*ConfigRecord, error) {
	return scanConfig(p.pool.QueryRow(ctx, `SELECT `+configCols+` FROM game_configs WHERE id=$1`, id))
}

func (p *PG) GetPublishedConfig(ctx context.Context) (*ConfigRecord, error) {
	return scanConfig(p.pool.QueryRow(ctx, `SELECT `+configCols+` FROM game_configs WHERE published LIMIT 1`))
}

func (p *PG) ListConfigs(ctx context.Context) ([]*ConfigRecord, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+configCols+` FROM game_configs ORDER BY version DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ConfigRecord
	for rows.Next() {
		c, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (p *PG) PublishConfig(ctx context.Context, id string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE game_configs SET published=FALSE WHERE published`); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE game_configs SET published=TRUE WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (p *PG) Close() error {
	p.pool.Close()
	return nil
}

var _ Store = (*PG)(nil)
var _ Store = (*Mem)(nil)
