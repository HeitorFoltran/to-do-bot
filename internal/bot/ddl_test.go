package bot

import (
	"strings"
	"testing"
	"time"

	"merge-bot/internal/store"
)

func TestDrafts(t *testing.T) {
	var d drafts
	id := d.put(draft{authorID: "a", reviewerID: "r"})
	if id <= 0 {
		t.Fatalf("id = %d, quer positivo", id)
	}
	if _, ok := d.take(id, "outro"); ok {
		t.Error("take aceitou outro autor")
	}
	dr, ok := d.take(id, "a")
	if !ok || dr.reviewerID != "r" {
		t.Fatalf("take = %+v, %v", dr, ok)
	}
	if _, ok := d.take(id, "a"); ok {
		t.Error("take devolveu o mesmo rascunho duas vezes")
	}

	id = d.put(draft{authorID: "a"})
	d.m[id] = draft{authorID: "a", expires: time.Now().Add(-time.Second)}
	if _, ok := d.take(id, "a"); ok {
		t.Error("take aceitou rascunho expirado")
	}
}

func TestPendingTextDDL(t *testing.T) {
	m := &store.Merge{ID: 7, GuildID: "g", ChannelID: "c", MessageID: "msg", HasDDL: true}
	want := "[no anúncio](https://discord.com/channels/g/c/msg)"
	if got := pendingText(m, time.UTC); !strings.Contains(got, want) {
		t.Errorf("pendingText sem link da DDL:\n%s", got)
	}
	m.HasDDL = false
	if got := pendingText(m, time.UTC); strings.Contains(got, "DDL") {
		t.Errorf("pendingText mostrou DDL sem ter:\n%s", got)
	}
}
