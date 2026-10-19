package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const (
	maxDDLLen   = 4000
	maxDDLBytes = 8 << 20
	draftTTL    = 15 * time.Minute
)

var httpClient = &http.Client{Timeout: 20 * time.Second}

// draft guarda o que o /merge recebeu até o modal de criação ser enviado.
type draft struct {
	authorID   string
	reviewerID string
	attachment *discordgo.MessageAttachment
	expires    time.Time
}

type drafts struct {
	mu sync.Mutex
	m  map[int64]draft
}

func (d *drafts) put(dr draft) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.m == nil {
		d.m = map[int64]draft{}
	}
	now := time.Now()
	for k, v := range d.m {
		if now.After(v.expires) {
			delete(d.m, k)
		}
	}
	dr.expires = now.Add(draftTTL)
	for {
		id := rand.Int64N(math.MaxInt64) + 1
		if _, taken := d.m[id]; !taken {
			d.m[id] = dr
			return id
		}
	}
}

func (d *drafts) take(id int64, authorID string) (draft, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	dr, ok := d.m[id]
	if !ok || dr.authorID != authorID {
		return draft{}, false
	}
	delete(d.m, id)
	if time.Now().After(dr.expires) {
		return draft{}, false
	}
	return dr, true
}

func ddlFile(mergeID int64, text string) *discordgo.File {
	return &discordgo.File{
		Name:        fmt.Sprintf("ddl-%d.sql", mergeID),
		ContentType: "text/plain; charset=utf-8",
		Reader:      strings.NewReader(text),
	}
}

func downloadAttachment(ctx context.Context, a *discordgo.MessageAttachment) (*discordgo.File, error) {
	if a.Size > maxDDLBytes {
		return nil, errTooBig
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDDLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDDLBytes {
		return nil, errTooBig
	}
	return &discordgo.File{Name: a.Filename, ContentType: a.ContentType, Reader: bytes.NewReader(data)}, nil
}

var errTooBig = errors.New("arquivo grande demais")
