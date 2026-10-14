// Package bot contém os slash commands, handlers e embeds do Discord.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"runtime/debug"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"merge-bot/internal/store"
)

// itemsPerMessage respeita os limites do Discord (10 embeds e 5 action rows).
const itemsPerMessage = 5

const dbTimeout = 5 * time.Second

type Handler struct {
	store *store.Store
	loc   *time.Location
	now   func() time.Time
}

func NewHandler(st *store.Store, loc *time.Location) *Handler {
	return &Handler{store: st, loc: loc, now: time.Now}
}

// OnInteraction é registrado com session.AddHandler. Um panic num handler
// é recuperado e logado para não derrubar o processo.
func (h *Handler) OnInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic no handler de interação", "panic", r, "stack", string(debug.Stack()))
			respondEphemeral(s, i, "Erro interno ao processar o comando.")
		}
	}()

	if i.Member == nil || i.GuildID == "" {
		respondEphemeral(s, i, "Use este comando dentro do servidor.")
		return
	}

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		switch name := i.ApplicationCommandData().Name; name {
		case cmdMerge:
			h.handleMerge(s, i)
		case cmdPendentes:
			h.handlePendentes(s, i)
		default:
			slog.Warn("comando desconhecido", "name", name)
			respondEphemeral(s, i, "Comando desconhecido.")
		}
	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID
		if strings.HasPrefix(customID, doneButtonPrefix) {
			h.handleDoneButton(s, i)
			return
		}
		slog.Warn("componente desconhecido", "custom_id", customID)
		respondEphemeral(s, i, "Ação desconhecida.")
	}
}

func (h *Handler) handleMerge(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	var taskURL, mergeURL, reviewerID, note string
	for _, opt := range data.Options {
		switch opt.Name {
		case optTarefa:
			taskURL = strings.TrimSpace(opt.StringValue())
		case optMerge:
			mergeURL = strings.TrimSpace(opt.StringValue())
		case optResponsavel:
			reviewerID = opt.UserValue(nil).ID
		case optObs:
			note = strings.TrimSpace(opt.StringValue())
		}
	}

	if err := validateURL(taskURL); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Link da tarefa inválido: %v", err))
		return
	}
	if err := validateURL(mergeURL); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Link do merge inválido: %v", err))
		return
	}
	if utf8.RuneCountInString(note) > maxNoteLen {
		respondEphemeral(s, i, fmt.Sprintf("A observação pode ter no máximo %d caracteres.", maxNoteLen))
		return
	}
	if reviewerID == "" {
		respondEphemeral(s, i, "Informe o responsável.")
		return
	}
	if u := resolvedUser(data, reviewerID); u != nil && u.Bot {
		respondEphemeral(s, i, "O responsável não pode ser um bot.")
		return
	}

	m := &store.Merge{
		GuildID:    i.GuildID,
		AuthorID:   i.Member.User.ID,
		ReviewerID: reviewerID,
		TaskURL:    taskURL,
		MergeURL:   mergeURL,
		Note:       note,
		ChannelID:  i.ChannelID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if _, err := h.store.Create(ctx, m); err != nil {
		slog.Error("criando merge", "err", err)
		respondEphemeral(s, i, "Não consegui salvar o merge. Tente de novo.")
		return
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("%s, %s abriu um merge para você revisar.", mention(reviewerID), mention(m.AuthorID)),
			Embeds:  []*discordgo.MessageEmbed{announcementEmbed(m)},
			// Só o responsável é notificado.
			AllowedMentions: &discordgo.MessageAllowedMentions{Users: []string{reviewerID}},
		},
	})
	if err != nil {
		// Sem anúncio o usuário vai tentar de novo; desfaz para não duplicar.
		slog.Error("respondendo /merge", "err", err, "merge_id", m.ID)
		if derr := h.store.Delete(ctx, m.ID); derr != nil {
			slog.Error("desfazendo merge", "err", derr, "merge_id", m.ID)
		}
		return
	}

	msg, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		slog.Warn("obtendo mensagem de anúncio", "err", err, "merge_id", m.ID)
		return
	}
	if err := h.store.SetMessageID(ctx, m.ID, msg.ChannelID, msg.ID); err != nil {
		slog.Warn("salvando message_id", "err", err, "merge_id", m.ID)
	}
	slog.Info("merge registrado", "merge_id", m.ID, "author", m.AuthorID, "reviewer", reviewerID)
}

func (h *Handler) handlePendentes(s *discordgo.Session, i *discordgo.InteractionCreate) {
	var reviewerID string
	if opt := i.ApplicationCommandData().GetOption(optResponsavel); opt != nil {
		reviewerID = opt.UserValue(nil).ID
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	merges, err := h.store.ListPending(ctx, i.GuildID, reviewerID)
	if err != nil {
		slog.Error("listando pendentes", "err", err)
		respondEphemeral(s, i, "Não consegui listar os merges. Tente de novo.")
		return
	}
	if len(merges) == 0 {
		respondEphemeral(s, i, "Nenhum merge pendente 🎉")
		return
	}

	now := h.now()
	noMentions := &discordgo.MessageAllowedMentions{}
	for n, group := range chunk(merges, itemsPerMessage) {
		embeds := make([]*discordgo.MessageEmbed, 0, len(group))
		rows := make([]discordgo.MessageComponent, 0, len(group))
		for k := range group {
			embeds = append(embeds, pendingEmbed(&group[k], now, h.loc))
			rows = append(rows, doneButtonRow(&group[k]))
		}

		if n == 0 {
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content:         pendingHeader(len(merges), reviewerID),
					Embeds:          embeds,
					Components:      rows,
					AllowedMentions: noMentions,
				},
			})
		} else {
			_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Embeds:          embeds,
				Components:      rows,
				AllowedMentions: noMentions,
			})
		}
		if err != nil {
			slog.Error("enviando lista de pendentes", "err", err, "page", n)
			return
		}
	}
}

func pendingHeader(total int, reviewerID string) string {
	var b strings.Builder
	if total == 1 {
		b.WriteString("**1 merge pendente**")
	} else {
		fmt.Fprintf(&b, "**%d merges pendentes**", total)
	}
	if reviewerID != "" {
		b.WriteString(" para " + mention(reviewerID))
	}
	return b.String()
}

func (h *Handler) handleDoneButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	id, ok := parseDoneButtonID(customID)
	if !ok {
		respondEphemeral(s, i, "Botão inválido.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	m, err := h.store.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && m.GuildID != i.GuildID) {
		h.respondAlreadyDone(s, i, customID)
		return
	}
	if err != nil {
		slog.Error("buscando merge", "err", err, "merge_id", id)
		respondEphemeral(s, i, "Não consegui concluir o merge. Tente de novo.")
		return
	}

	user := i.Member.User
	if user.ID != m.ReviewerID && !isManager(i.Member) {
		respondEphemeral(s, i, fmt.Sprintf("Só %s ou um administrador pode concluir esse merge.", mention(m.ReviewerID)))
		return
	}

	if err := h.store.Delete(ctx, id); errors.Is(err, store.ErrNotFound) {
		h.respondAlreadyDone(s, i, customID)
		return
	} else if err != nil {
		slog.Error("excluindo merge", "err", err, "merge_id", id)
		respondEphemeral(s, i, "Não consegui concluir o merge. Tente de novo.")
		return
	}
	slog.Info("merge concluído", "merge_id", id, "by", user.ID)

	name := i.Member.DisplayName()
	if err := updateClickedMessage(s, i, customID, "✅ Mergeado por "+name); err != nil {
		slog.Error("atualizando mensagem da lista", "err", err, "merge_id", id)
	}

	if m.MessageID != "" {
		edit := discordgo.NewMessageEdit(m.ChannelID, m.MessageID).
			SetEmbeds([]*discordgo.MessageEmbed{announcementDoneEmbed(m, mention(user.ID))})
		edit.AllowedMentions = &discordgo.MessageAllowedMentions{}
		if _, err := s.ChannelMessageEditComplex(edit); err != nil {
			slog.Warn("editando anúncio original", "err", err, "merge_id", id)
		}
	}
}

// respondAlreadyDone desabilita o botão clicado e avisa, em ephemeral, que o
// merge já foi concluído.
func (h *Handler) respondAlreadyDone(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	if err := updateClickedMessage(s, i, customID, "✅ Concluído"); err != nil {
		slog.Error("desabilitando botão", "err", err)
		respondEphemeral(s, i, "Esse merge já foi concluído")
		return
	}
	if _, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: "Esse merge já foi concluído",
		Flags:   discordgo.MessageFlagsEphemeral,
	}); err != nil {
		slog.Warn("enviando aviso de já concluído", "err", err)
	}
}

// updateClickedMessage responde a interação alterando só o botão clicado.
func updateClickedMessage(s *discordgo.Session, i *discordgo.InteractionCreate, customID, label string) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:         i.Message.Content,
			Embeds:          i.Message.Embeds,
			Components:      disableButton(i.Message.Components, customID, label),
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	})
}

func isManager(m *discordgo.Member) bool {
	return m.Permissions&(discordgo.PermissionAdministrator|discordgo.PermissionManageGuild) != 0
}

func validateURL(raw string) error {
	if raw == "" {
		return errors.New("vazio")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return errors.New("não é uma URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("precisa começar com http:// ou https://")
	}
	if u.Host == "" {
		return errors.New("sem domínio")
	}
	return nil
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:         content,
			Flags:           discordgo.MessageFlagsEphemeral,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	})
	if err != nil {
		slog.Error("respondendo interação", "err", err)
	}
}

func resolvedUser(data discordgo.ApplicationCommandInteractionData, userID string) *discordgo.User {
	if data.Resolved == nil {
		return nil
	}
	return data.Resolved.Users[userID]
}
