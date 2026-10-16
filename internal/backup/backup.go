package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"merge-bot/internal/store"
)

const filePrefix = "merges-"

type Backup struct {
	store *store.Store
	dir   string
	keep  int
	loc   *time.Location
	now   func() time.Time
}

func New(st *store.Store, dir string, keep int, loc *time.Location) *Backup {
	return &Backup{store: st, dir: dir, keep: keep, loc: loc, now: time.Now}
}

func (b *Backup) Run(ctx context.Context) (string, error) {
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return "", fmt.Errorf("criando pasta de backup: %w", err)
	}
	path := filepath.Join(b.dir, filePrefix+b.now().In(b.loc).Format("2006-01-02_15-04-05")+".db")
	if err := b.store.Backup(ctx, path); err != nil {
		return "", err
	}
	if err := prune(b.dir, b.keep); err != nil {
		return path, err
	}
	return path, nil
}

func prune(dir string, keep int) error {
	files, err := filepath.Glob(filepath.Join(dir, filePrefix+"*.db"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for len(files) > keep {
		if err := os.Remove(files[0]); err != nil {
			return fmt.Errorf("removendo backup antigo: %w", err)
		}
		files = files[1:]
	}
	return nil
}
