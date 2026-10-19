package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("merge não encontrado")

const schema = `
CREATE TABLE IF NOT EXISTS merges (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id        TEXT    NOT NULL,
    author_id       TEXT    NOT NULL,
    reviewer_id     TEXT    NOT NULL,
    task_url        TEXT    NOT NULL,
    merge_url       TEXT    NOT NULL,
    note            TEXT,
    channel_id      TEXT    NOT NULL,
    message_id      TEXT,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_merges_reviewer ON merges(guild_id, reviewer_id);

CREATE TABLE IF NOT EXISTS guild_settings (
    guild_id         TEXT    PRIMARY KEY,
    reminder_cron    TEXT    NOT NULL,
    timezone         TEXT    NOT NULL,
    reminder_enabled INTEGER NOT NULL
);
`

type Merge struct {
	ID         int64
	GuildID    string
	AuthorID   string
	ReviewerID string
	TaskURLs   []string
	MergeURLs  []string
	Note       string
	ChannelID  string
	MessageID  string
	HasDDL     bool
	CreatedAt  time.Time
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrindo sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrando schema: %w", err)
	}
	if err := addColumn(db, "merges", "has_ddl", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func addColumn(db *sql.DB, table, column, def string) error {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n)
	if err != nil {
		return fmt.Errorf("verificando coluna %s.%s: %w", table, column, err)
	}
	if n > 0 {
		return nil
	}
	if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, def)); err != nil {
		return fmt.Errorf("adicionando coluna %s.%s: %w", table, column, err)
	}
	return nil
}

func (s *Store) Backup(ctx context.Context, path string) error {
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("gerando backup em %s: %w", path, err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Create(ctx context.Context, m *Merge) (int64, error) {
	createdAt := m.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO merges (guild_id, author_id, reviewer_id, task_url, merge_url, note, channel_id, message_id, has_ddl, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.GuildID, m.AuthorID, m.ReviewerID, joinURLs(m.TaskURLs), joinURLs(m.MergeURLs),
		nullString(m.Note), m.ChannelID, nullString(m.MessageID), m.HasDDL, formatTime(createdAt),
	)
	if err != nil {
		return 0, fmt.Errorf("inserindo merge: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("obtendo id: %w", err)
	}
	m.ID = id
	m.CreatedAt = createdAt.UTC().Truncate(time.Second)
	return id, nil
}

func (s *Store) Get(ctx context.Context, id int64) (*Merge, error) {
	row := s.db.QueryRowContext(ctx, selectCols+` WHERE id = ?`, id)
	m, err := scanMerge(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscando merge %d: %w", id, err)
	}
	return m, nil
}

type Settings struct {
	GuildID         string
	ReminderCron    string
	Timezone        string
	ReminderEnabled bool
}

func (s *Store) GetSettings(ctx context.Context, guildID string) (*Settings, error) {
	st := Settings{GuildID: guildID}
	err := s.db.QueryRowContext(ctx,
		`SELECT reminder_cron, timezone, reminder_enabled FROM guild_settings WHERE guild_id = ?`, guildID,
	).Scan(&st.ReminderCron, &st.Timezone, &st.ReminderEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("buscando configuração do servidor %s: %w", guildID, err)
	}
	return &st, nil
}

func (s *Store) SaveSettings(ctx context.Context, st *Settings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO guild_settings (guild_id, reminder_cron, timezone, reminder_enabled)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (guild_id) DO UPDATE SET
			reminder_cron = excluded.reminder_cron,
			timezone = excluded.timezone,
			reminder_enabled = excluded.reminder_enabled`,
		st.GuildID, st.ReminderCron, st.Timezone, st.ReminderEnabled)
	if err != nil {
		return fmt.Errorf("gravando configuração do servidor %s: %w", st.GuildID, err)
	}
	return nil
}

type Filter struct {
	ReviewerID string
	AuthorID   string
}

func (s *Store) ListPending(ctx context.Context, guildID string, f Filter) ([]Merge, error) {
	query := selectCols + ` WHERE guild_id = ?`
	args := []any{guildID}
	if f.ReviewerID != "" {
		query += ` AND reviewer_id = ?`
		args = append(args, f.ReviewerID)
	}
	if f.AuthorID != "" {
		query += ` AND author_id = ?`
		args = append(args, f.AuthorID)
	}
	query += ` ORDER BY created_at ASC, id ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listando merges: %w", err)
	}
	defer rows.Close()

	var out []Merge
	for rows.Next() {
		m, err := scanMerge(rows)
		if err != nil {
			return nil, fmt.Errorf("lendo merge: %w", err)
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM merges WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("excluindo merge %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("excluindo merge %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Update(ctx context.Context, m *Merge) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE merges SET reviewer_id = ?, task_url = ?, merge_url = ?, note = ?, has_ddl = ? WHERE id = ?`,
		m.ReviewerID, joinURLs(m.TaskURLs), joinURLs(m.MergeURLs), nullString(m.Note), m.HasDDL, m.ID)
	if err != nil {
		return fmt.Errorf("atualizando merge %d: %w", m.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetMessageID(ctx context.Context, id int64, channelID, messageID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE merges SET channel_id = ?, message_id = ? WHERE id = ?`, channelID, messageID, id)
	if err != nil {
		return fmt.Errorf("gravando message_id do merge %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const selectCols = `SELECT id, guild_id, author_id, reviewer_id, task_url, merge_url,
	COALESCE(note, ''), channel_id, COALESCE(message_id, ''), has_ddl, created_at FROM merges`

type scanner interface {
	Scan(dest ...any) error
}

func scanMerge(sc scanner) (*Merge, error) {
	var m Merge
	var taskURLs, mergeURLs, createdAt string
	if err := sc.Scan(&m.ID, &m.GuildID, &m.AuthorID, &m.ReviewerID, &taskURLs, &mergeURLs,
		&m.Note, &m.ChannelID, &m.MessageID, &m.HasDDL, &createdAt); err != nil {
		return nil, err
	}
	m.TaskURLs = splitURLs(taskURLs)
	m.MergeURLs = splitURLs(mergeURLs)
	t, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	m.CreatedAt = t
	return &m, nil
}

const timeLayout = "2006-01-02 15:04:05"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{timeLayout, time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("created_at inválido: %q", s)
}

func joinURLs(urls []string) string {
	return strings.Join(urls, "\n")
}

func splitURLs(s string) []string {
	return strings.Fields(s)
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
