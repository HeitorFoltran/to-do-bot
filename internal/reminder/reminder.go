// Package reminder posta o lembrete semanal de merges pendentes.
package reminder

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"merge-bot/internal/bot"
	"merge-bot/internal/store"
)

// maxMessageLen é o limite de caracteres de uma mensagem do Discord.
const maxMessageLen = 2000

type Reminder struct {
	session   *discordgo.Session
	store     *store.Store
	guildID   string
	channelID string
	now       func() time.Time
}

func New(s *discordgo.Session, st *store.Store, guildID, channelID string) *Reminder {
	return &Reminder{session: s, store: st, guildID: guildID, channelID: channelID, now: time.Now}
}

// Run busca os pendentes e posta o lembrete. Sem pendentes, não posta nada.
func (r *Reminder) Run(ctx context.Context) {
	merges, err := r.store.ListPending(ctx, r.guildID, "")
	if err != nil {
		slog.Error("lembrete: listando pendentes", "err", err)
		return
	}
	if len(merges) == 0 {
		slog.Info("lembrete: nenhum pendente, nada a postar")
		return
	}

	messages, reviewers := BuildMessages(merges, r.now())
	for _, content := range messages {
		_, err := r.session.ChannelMessageSendComplex(r.channelID, &discordgo.MessageSend{
			Content: content,
			// Notifica só os responsáveis, não os autores.
			AllowedMentions: &discordgo.MessageAllowedMentions{Users: reviewers},
		}, discordgo.WithContext(ctx))
		if err != nil {
			slog.Error("lembrete: enviando mensagem", "err", err)
			return
		}
	}
	slog.Info("lembrete postado", "pendentes", len(merges), "responsaveis", len(reviewers))
}

// BuildMessages monta o texto do lembrete agrupado por responsável (na ordem
// do pendente mais antigo de cada um), quebrado em mensagens de até 2000
// caracteres. Também devolve os IDs dos responsáveis mencionados.
func BuildMessages(merges []store.Merge, now time.Time) (messages []string, reviewers []string) {
	byReviewer := map[string][]store.Merge{}
	for _, m := range merges {
		if _, ok := byReviewer[m.ReviewerID]; !ok {
			reviewers = append(reviewers, m.ReviewerID)
		}
		byReviewer[m.ReviewerID] = append(byReviewer[m.ReviewerID], m)
	}

	lines := []string{"📋 **Lembrete semanal: merges pendentes**"}
	for _, reviewerID := range reviewers {
		items := byReviewer[reviewerID]
		count := "1 pendente"
		if len(items) > 1 {
			count = fmt.Sprintf("%d pendentes", len(items))
		}
		lines = append(lines, "", fmt.Sprintf("<@%s>: %s", reviewerID, count))
		for _, m := range items {
			// <> evita o preview do link.
			lines = append(lines, fmt.Sprintf("• #%d de <@%s>: <%s> (aberto %s)",
				m.ID, m.AuthorID, m.MergeURL, bot.Age(m.CreatedAt, now)))
		}
	}
	lines = append(lines, "", "Para dar baixa, use `/pendentes`.")

	return splitLines(lines, maxMessageLen), reviewers
}

// splitLines junta as linhas em blocos de até limit caracteres sem quebrar
// uma linha no meio (linhas maiores que limit são truncadas).
func splitLines(lines []string, limit int) []string {
	var out []string
	var b strings.Builder
	for _, line := range lines {
		if len(line) > limit {
			line = line[:limit]
		}
		if b.Len() > 0 && b.Len()+1+len(line) > limit {
			out = append(out, b.String())
			b.Reset()
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	if strings.TrimSpace(b.String()) != "" {
		out = append(out, b.String())
	}
	return out
}
