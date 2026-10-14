package reminder

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"merge-bot/internal/store"
)

func TestBuildMessagesGroupsByReviewer(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	merges := []store.Merge{
		{ID: 1, ReviewerID: "r1", AuthorID: "a1", MergeURL: "https://git/1", CreatedAt: now.Add(-72 * time.Hour)},
		{ID: 2, ReviewerID: "r2", AuthorID: "a2", MergeURL: "https://git/2", CreatedAt: now.Add(-48 * time.Hour)},
		{ID: 3, ReviewerID: "r1", AuthorID: "a2", MergeURL: "https://git/3", CreatedAt: now.Add(-3 * time.Hour)},
	}

	msgs, reviewers := BuildMessages(merges, now)
	if len(msgs) != 1 {
		t.Fatalf("esperava 1 mensagem, veio %d", len(msgs))
	}
	if strings.Join(reviewers, ",") != "r1,r2" {
		t.Errorf("reviewers = %v", reviewers)
	}

	msg := msgs[0]
	for _, want := range []string{
		"<@r1>: 2 pendentes",
		"<@r2>: 1 pendente",
		"• #1 de <@a1>: <https://git/1> (aberto há 3 dias)",
		"• #3 de <@a2>: <https://git/3> (aberto há 3 horas)",
		"• #2 de <@a2>: <https://git/2> (aberto há 2 dias)",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("mensagem não contém %q:\n%s", want, msg)
		}
	}
	// #3 (de r1) vem antes do bloco de r2.
	if strings.Index(msg, "#3") > strings.Index(msg, "<@r2>") {
		t.Errorf("itens de r1 deveriam vir juntos antes de r2:\n%s", msg)
	}
}

func TestBuildMessagesSplitsLongOutput(t *testing.T) {
	now := time.Now()
	var merges []store.Merge
	for k := 1; k <= 100; k++ {
		merges = append(merges, store.Merge{
			ID: int64(k), ReviewerID: fmt.Sprintf("r%d", k%7), AuthorID: "a",
			MergeURL: "https://gitlab.example.com/group/project/-/merge_requests/" + fmt.Sprint(k), CreatedAt: now,
		})
	}
	msgs, _ := BuildMessages(merges, now)
	if len(msgs) < 2 {
		t.Fatalf("esperava várias mensagens, veio %d", len(msgs))
	}
	total := 0
	for _, m := range msgs {
		if len(m) > maxMessageLen {
			t.Errorf("mensagem com %d caracteres", len(m))
		}
		total += strings.Count(m, "• #")
	}
	if total != 100 {
		t.Errorf("itens no total = %d, quer 100", total)
	}
}
