package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func sample(guild, reviewer string, createdAt time.Time) *Merge {
	return &Merge{
		GuildID:    guild,
		AuthorID:   "author",
		ReviewerID: reviewer,
		TaskURLs:   []string{"https://tasks.example.com/1"},
		MergeURLs:  []string{"https://git.example.com/mr/1", "https://git.example.com/mr/2"},
		ChannelID:  "chan",
		CreatedAt:  createdAt,
	}
}

func TestCreateAndGet(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	created := time.Date(2026, 10, 1, 12, 30, 15, 0, time.UTC)
	in := sample("g1", "r1", created)
	in.Note = "urgente"
	id, err := st.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == 0 || in.ID != id {
		t.Fatalf("id = %d, in.ID = %d", id, in.ID)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.GuildID != "g1" || got.ReviewerID != "r1" || got.AuthorID != "author" ||
		!slices.Equal(got.TaskURLs, in.TaskURLs) || !slices.Equal(got.MergeURLs, in.MergeURLs) || got.Note != "urgente" ||
		got.ChannelID != "chan" || got.MessageID != "" {
		t.Errorf("Get devolveu %+v", got)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, quer %v", got.CreatedAt, created)
	}
}

func TestCreateDefaultsCreatedAt(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	before := time.Now().UTC().Truncate(time.Second)
	id, err := st.Create(ctx, sample("g1", "r1", time.Time{}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CreatedAt.Before(before) || got.CreatedAt.After(time.Now().Add(time.Second)) {
		t.Errorf("CreatedAt = %v, esperado perto de agora", got.CreatedAt)
	}
}

func TestGetNotFound(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Get(context.Background(), 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, quer ErrNotFound", err)
	}
}

func TestListPendingOrderAndFilter(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	mustCreate(t, st, sample("g1", "r1", base.Add(2*time.Hour)))
	mustCreate(t, st, sample("g1", "r2", base))
	mustCreate(t, st, sample("g1", "r1", base.Add(time.Hour)))
	mustCreate(t, st, sample("g2", "r1", base.Add(-time.Hour)))
	mustCreate(t, st, sample("g1", "r2", base.Add(2*time.Hour)))

	all, err := st.ListPending(ctx, "g1", Filter{})
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	assertIDs(t, all, 2, 3, 1, 5)

	r1, err := st.ListPending(ctx, "g1", Filter{ReviewerID: "r1"})
	if err != nil {
		t.Fatalf("ListPending r1: %v", err)
	}
	assertIDs(t, r1, 3, 1)

	none, err := st.ListPending(ctx, "g1", Filter{ReviewerID: "ninguem"})
	if err != nil {
		t.Fatalf("ListPending vazio: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("esperava lista vazia, veio %d", len(none))
	}
}

func TestListPendingFilterByAuthor(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	withAuthor := func(author, reviewer string, at time.Time) *Merge {
		m := sample("g1", reviewer, at)
		m.AuthorID = author
		return m
	}
	mustCreate(t, st, withAuthor("a1", "r1", base))
	mustCreate(t, st, withAuthor("a2", "r1", base.Add(time.Hour)))
	mustCreate(t, st, withAuthor("a1", "r2", base.Add(2*time.Hour)))
	mustCreate(t, st, withAuthor("a1", "r1", base.Add(3*time.Hour)))

	a1, err := st.ListPending(ctx, "g1", Filter{AuthorID: "a1"})
	if err != nil {
		t.Fatalf("ListPending a1: %v", err)
	}
	assertIDs(t, a1, 1, 3, 4)

	both, err := st.ListPending(ctx, "g1", Filter{ReviewerID: "r1", AuthorID: "a1"})
	if err != nil {
		t.Fatalf("ListPending r1+a1: %v", err)
	}
	assertIDs(t, both, 1, 4)
}

func TestDelete(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id := mustCreate(t, st, sample("g1", "r1", time.Now()))

	if err := st.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := st.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get após Delete: err = %v", err)
	}
	if err := st.Delete(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("segundo Delete: err = %v, quer ErrNotFound", err)
	}
}

func TestSetMessageID(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id := mustCreate(t, st, sample("g1", "r1", time.Now()))

	if err := st.SetMessageID(ctx, id, "chan2", "msg1"); err != nil {
		t.Fatalf("SetMessageID: %v", err)
	}
	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ChannelID != "chan2" || got.MessageID != "msg1" {
		t.Errorf("channel/message = %q/%q", got.ChannelID, got.MessageID)
	}
	if err := st.SetMessageID(ctx, 999, "c", "m"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetMessageID inexistente: err = %v", err)
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	path := t.TempDir() + "/merges.db"
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	id := mustCreate(t, st, sample("g1", "r1", time.Now()))
	st.Close()

	st, err = Open(path)
	if err != nil {
		t.Fatalf("reabrindo: %v", err)
	}
	defer st.Close()
	if _, err := st.Get(context.Background(), id); err != nil {
		t.Fatalf("Get após reabrir: %v", err)
	}
}

func mustCreate(t *testing.T, st *Store, m *Merge) int64 {
	t.Helper()
	id, err := st.Create(context.Background(), m)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return id
}

func assertIDs(t *testing.T, got []Merge, want ...int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, quer %d (%+v)", len(got), len(want), got)
	}
	for k := range want {
		if got[k].ID != want[k] {
			t.Errorf("posição %d: id %d, quer %d", k, got[k].ID, want[k])
		}
	}
}

func TestSettings(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if _, err := st.GetSettings(ctx, "g1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSettings sem registro: err = %v", err)
	}

	in := &Settings{GuildID: "g1", ReminderCron: "0 9 * * 3", Timezone: "America/Sao_Paulo", ReminderEnabled: true}
	if err := st.SaveSettings(ctx, in); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	in.ReminderCron, in.ReminderEnabled = "0 10 * * 1", false
	if err := st.SaveSettings(ctx, in); err != nil {
		t.Fatalf("SaveSettings (update): %v", err)
	}
	if err := st.SaveSettings(ctx, &Settings{GuildID: "g2", ReminderCron: "0 8 * * 5", Timezone: "UTC", ReminderEnabled: true}); err != nil {
		t.Fatalf("SaveSettings g2: %v", err)
	}

	got, err := st.GetSettings(ctx, "g1")
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if *got != *in {
		t.Errorf("GetSettings = %+v, quer %+v", got, in)
	}
	g2, err := st.GetSettings(ctx, "g2")
	if err != nil || g2.Timezone != "UTC" {
		t.Errorf("g2 = %+v, err = %v", g2, err)
	}
}

func TestUpdate(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	m := sample("g1", "r1", time.Now())
	m.Note = "antes"
	id := mustCreate(t, st, m)

	m.ReviewerID = "r2"
	m.TaskURLs = []string{"https://t/novo"}
	m.MergeURLs = []string{"https://m/1", "https://m/2"}
	m.Note = ""
	if err := st.Update(ctx, m); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ReviewerID != "r2" || !slices.Equal(got.TaskURLs, m.TaskURLs) ||
		!slices.Equal(got.MergeURLs, m.MergeURLs) || got.Note != "" || got.AuthorID != "author" {
		t.Errorf("depois do Update: %+v", got)
	}

	if err := st.Update(ctx, &Merge{ID: 999}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update inexistente: err = %v", err)
	}
}
