package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"merge-bot/internal/store"
)

func TestRunCreatesAndPrunes(t *testing.T) {
	src, err := store.Open(filepath.Join(t.TempDir(), "merges.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	ctx := context.Background()
	if _, err := src.Create(ctx, &store.Merge{GuildID: "g", AuthorID: "a", ReviewerID: "r", ChannelID: "c", MergeURLs: []string{"https://m/1"}}); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "backups")
	b := New(src, dir, 3, time.UTC)
	clock := time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return clock }

	var last string
	for range 5 {
		if last, err = b.Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}
		clock = clock.Add(24 * time.Hour)
	}

	files, _ := filepath.Glob(filepath.Join(dir, "merges-*.db"))
	if len(files) != 3 {
		t.Fatalf("esperava 3 backups, há %d: %v", len(files), files)
	}
	if filepath.Base(files[0]) != "merges-2026-10-18_03-00-00.db" || files[2] != last {
		t.Errorf("backups mantidos = %v (último %s)", files, last)
	}

	restored, err := store.Open(last)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	ms, err := restored.ListPending(ctx, "g", store.Filter{})
	if err != nil || len(ms) != 1 {
		t.Errorf("backup sem o merge: %v, %v", ms, err)
	}
}

func TestRunFailsOnUnwritableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root escreve em qualquer pasta")
	}
	src, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if _, err := New(src, dir, 3, time.UTC).Run(context.Background()); err == nil {
		t.Error("Run deveria falhar numa pasta sem permissão de escrita")
	}
}
