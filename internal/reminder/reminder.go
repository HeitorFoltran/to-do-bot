package reminder

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"merge-bot/internal/store"
)

type Reminder struct {
	session *discordgo.Session
	store   *store.Store
}

func New(s *discordgo.Session, st *store.Store) *Reminder {
	return &Reminder{session: s, store: st}
}

type Options struct {
	UserID  string
	Message string
}

type Result struct {
	Pending   int
	Reviewers int
	Failed    int
}

func (r *Reminder) Run(ctx context.Context, guildID string, opt Options) (Result, error) {
	merges, err := r.store.ListPending(ctx, guildID, store.Filter{ReviewerID: opt.UserID})
	if err != nil {
		return Result{}, fmt.Errorf("listando pendentes: %w", err)
	}
	res := Result{Pending: len(merges)}

	reviewers, byReviewer := groupByReviewer(merges)

	if opt.UserID != "" && opt.Message != "" && len(reviewers) == 0 {
		reviewers = []string{opt.UserID}
	}
	if len(reviewers) == 0 {
		slog.Info("lembrete: nenhum pendente, nada a enviar", "guild_id", guildID, "usuario", opt.UserID)
		return res, nil
	}

	res.Reviewers = len(reviewers)
	for _, reviewerID := range reviewers {
		if err := r.sendDM(ctx, reviewerID, messageFor(len(byReviewer[reviewerID]), opt.Message)); err != nil {
			slog.Warn("lembrete: DM falhou", "err", err, "guild_id", guildID, "reviewer", reviewerID)
			res.Failed++
		}
	}
	slog.Info("lembrete enviado", "guild_id", guildID, "pendentes", res.Pending,
		"responsaveis", res.Reviewers, "falhas", res.Failed)
	return res, nil
}

func (r *Reminder) sendDM(ctx context.Context, userID, content string) error {
	ch, err := r.session.UserChannelCreate(userID, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("abrindo DM: %w", err)
	}
	_, err = r.session.ChannelMessageSendComplex(ch.ID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	}, discordgo.WithContext(ctx))
	return err
}

func groupByReviewer(merges []store.Merge) ([]string, map[string][]store.Merge) {
	var reviewers []string
	byReviewer := map[string][]store.Merge{}
	for _, m := range merges {
		if _, ok := byReviewer[m.ReviewerID]; !ok {
			reviewers = append(reviewers, m.ReviewerID)
		}
		byReviewer[m.ReviewerID] = append(byReviewer[m.ReviewerID], m)
	}
	return reviewers, byReviewer
}

const QuantityPlaceholder = "{quantidade}"

func messageFor(n int, custom string) string {
	if custom == "" {
		return dmText(n)
	}
	return strings.ReplaceAll(custom, QuantityPlaceholder, strconv.Itoa(n))
}

func dmText(n int) string {
	if n == 1 {
		return "**Você tem 1 merge pendente de aprovação.**"
	}
	return fmt.Sprintf("**Você tem %d merges pendentes de aprovação.**", n)
}
