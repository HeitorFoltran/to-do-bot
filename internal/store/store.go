// Package store faz o acesso ao SQLite onde ficam os merges pendentes.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound indica que o merge não existe (ou já foi concluído).
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
`

// Merge é um merge request pendente de revisão.
type Merge struct {
	ID         int64
	GuildID    string
	AuthorID   string
	ReviewerID string
	TaskURL    string
	MergeURL   string
	Note       string
	ChannelID  string
	MessageID  string
	CreatedAt  time.Time
}

type Store struct {
	db *sql.DB
}

// Open abre (ou cria) o banco em path e aplica a migração.
// Use ":memory:" para um banco em memória.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrindo sqlite: %w", err)
	}
	// Uma conexão só: evita SQLITE_BUSY e mantém o mesmo banco no modo :memory:.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrando schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Create insere o merge e devolve o id gerado. CreatedAt zero vira o horário atual.
func (s *Store) Create(ctx context.Context, m *Merge) (int64, error) {
	createdAt := m.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO merges (guild_id, author_id, reviewer_id, task_url, merge_url, note, channel_id, message_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.GuildID, m.AuthorID, m.ReviewerID, m.TaskURL, m.MergeURL,
		nullString(m.Note), m.ChannelID, nullString(m.MessageID), formatTime(createdAt),
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

// Get busca um merge pelo id. Retorna ErrNotFound se não existir.
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

// ListPending lista os merges do servidor, do mais antigo ao mais novo.
// reviewerID vazio lista todos.
func (s *Store) ListPending(ctx context.Context, guildID, reviewerID string) ([]Merge, error) {
	query := selectCols + ` WHERE guild_id = ?`
	args := []any{guildID}
	if reviewerID != "" {
		query += ` AND reviewer_id = ?`
		args = append(args, reviewerID)
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

// Delete exclui o merge. Retorna ErrNotFound se ele já não existia.
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

// SetMessageID grava a mensagem de anúncio do merge.
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
	COALESCE(note, ''), channel_id, COALESCE(message_id, ''), created_at FROM merges`

type scanner interface {
	Scan(dest ...any) error
}

func scanMerge(sc scanner) (*Merge, error) {
	var m Merge
	var createdAt string
	if err := sc.Scan(&m.ID, &m.GuildID, &m.AuthorID, &m.ReviewerID, &m.TaskURL, &m.MergeURL,
		&m.Note, &m.ChannelID, &m.MessageID, &createdAt); err != nil {
		return nil, err
	}
	t, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	m.CreatedAt = t
	return &m, nil
}

// Datas ficam no formato do CURRENT_TIMESTAMP do SQLite (UTC), que ordena
// corretamente como texto.
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

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
