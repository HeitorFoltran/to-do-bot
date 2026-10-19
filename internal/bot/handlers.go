package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"merge-bot/internal/reminder"
	"merge-bot/internal/store"
)

const (
	itemsPerMessage = 5
	maxPageChars    = 3500
)

const dbTimeout = 5 * time.Second

type Handler struct {
	store    *store.Store
	rem      *reminder.Reminder
	sched    *reminder.Scheduler
	defaults store.Settings

	registered sync.Map
	drafts     drafts
}

func NewHandler(st *store.Store, rem *reminder.Reminder, sched *reminder.Scheduler, defaults store.Settings) *Handler {
	return &Handler{store: st, rem: rem, sched: sched, defaults: defaults}
}

func (h *Handler) settings(ctx context.Context, guildID string) (store.Settings, error) {
	return reminder.LoadSettings(ctx, h.store, guildID, h.defaults)
}

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
		case cmdComoEstamos:
			h.handleComoEstamos(s, i)
		case cmdConfig:
			h.handleConfig(s, i)
		case cmdHelp:
			respondEphemeral(s, i, helpText())
		default:
			slog.Warn("comando desconhecido", "name", name)
			respondEphemeral(s, i, "Comando desconhecido.")
		}
	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID
		action, id, ok := parseActionID(customID)
		if !ok {
			slog.Warn("componente desconhecido", "custom_id", customID)
			respondEphemeral(s, i, "Ação desconhecida.")
			return
		}
		switch action {
		case actDone:
			h.handleDoneButton(s, i, id)
		case actEdit:
			h.handleEditButton(s, i, id)
		case actEditLinks:
			h.handleEditLinksButton(s, i, id)
		case actEditReviewer:
			h.handleEditReviewer(s, i, id)
		case actCancel:
			h.handleCancelButton(s, i, id)
		case actCancelConfirm:
			h.handleCancelConfirm(s, i, id)
		default:
			slog.Warn("ação desconhecida", "custom_id", customID)
			respondEphemeral(s, i, "Ação desconhecida.")
		}
	case discordgo.InteractionModalSubmit:
		customID := i.ModalSubmitData().CustomID
		if action, id, ok := parseActionID(customID); ok {
			switch action {
			case actCreateModal:
				h.handleCreateModal(s, i, id)
				return
			case actEditModal:
				h.handleEditModal(s, i, id)
				return
			}
		}
		slog.Warn("modal desconhecido", "custom_id", customID)
		respondEphemeral(s, i, "Ação desconhecida.")
	}
}

func (h *Handler) handleMerge(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	reviewerID := userOption(i, optResponsavel)
	if reviewerID == "" {
		respondEphemeral(s, i, "Informe o responsável.")
		return
	}
	if u := resolvedUser(data, reviewerID); u != nil && u.Bot {
		respondEphemeral(s, i, "O responsável não pode ser um bot.")
		return
	}
	dr := draft{authorID: i.Member.User.ID, reviewerID: reviewerID}
	if opt := optionByName(data.Options, optDDL); opt != nil && data.Resolved != nil {
		dr.attachment = data.Resolved.Attachments[opt.StringValue()]
		if dr.attachment != nil && dr.attachment.Size > maxDDLBytes {
			respondEphemeral(s, i, fmt.Sprintf("O arquivo da DDL pode ter no máximo %d MB.", maxDDLBytes>>20))
			return
		}
	}
	ddlLabel := "DDL (opcional, se não anexou arquivo)"
	if dr.attachment != nil {
		ddlLabel = "DDL extra (o arquivo já vai anexado)"
	}
	modal := linksModal(actionID(actCreateModal, h.drafts.put(dr)), "Novo merge", ddlLabel, nil, nil, "")
	if err := s.InteractionRespond(i.Interaction, modal); err != nil {
		slog.Error("abrindo modal de criação", "err", err)
	}
}

func (h *Handler) handleCreateModal(s *discordgo.Session, i *discordgo.InteractionCreate, draftID int64) {
	dr, ok := h.drafts.take(draftID, i.Member.User.ID)
	if !ok {
		respondEphemeral(s, i, "Esse formulário expirou. Use /merge de novo.")
		return
	}
	form, problem := parseLinksModal(i)
	if problem != "" {
		respondEphemeral(s, i, problem)
		return
	}

	// Baixar o anexo pode passar dos 3 s que o Discord dá para responder.
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		slog.Error("adiando resposta do modal de criação", "err", err)
		return
	}
	fail := func(content string) {
		if err := s.InteractionResponseDelete(i.Interaction); err != nil {
			slog.Warn("apagando resposta adiada", "err", err)
		}
		if _, err := s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
			Content:         content,
			Flags:           discordgo.MessageFlagsEphemeral,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		}); err != nil {
			slog.Error("avisando erro de criação", "err", err)
		}
	}

	var files []*discordgo.File
	if dr.attachment != nil {
		dlCtx, dlCancel := context.WithTimeout(context.Background(), 30*time.Second)
		f, err := downloadAttachment(dlCtx, dr.attachment)
		dlCancel()
		if errors.Is(err, errTooBig) {
			fail(fmt.Sprintf("O arquivo da DDL pode ter no máximo %d MB.", maxDDLBytes>>20))
			return
		}
		if err != nil {
			slog.Error("baixando anexo da DDL", "err", err, "arquivo", dr.attachment.Filename)
			fail("Não consegui baixar o arquivo da DDL. Tente de novo.")
			return
		}
		files = append(files, f)
	}

	m := &store.Merge{
		GuildID:    i.GuildID,
		AuthorID:   i.Member.User.ID,
		ReviewerID: dr.reviewerID,
		TaskURLs:   form.taskURLs,
		MergeURLs:  form.mergeURLs,
		Note:       form.note,
		ChannelID:  i.ChannelID,
		HasDDL:     dr.attachment != nil || form.ddl != "",
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if _, err := h.store.Create(ctx, m); err != nil {
		slog.Error("criando merge", "err", err)
		fail("Não consegui salvar o merge. Tente de novo.")
		return
	}
	if form.ddl != "" {
		files = append(files, ddlFile(m.ID, form.ddl))
	}

	embeds := []*discordgo.MessageEmbed{announcementEmbed(m)}
	components := announcementComponents(m, "")
	msg, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:          &embeds,
		Components:      &components,
		Files:           files,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	})
	if err != nil {
		slog.Error("respondendo modal de criação", "err", err, "merge_id", m.ID)
		if derr := h.store.Delete(ctx, m.ID); derr != nil {
			slog.Error("desfazendo merge", "err", derr, "merge_id", m.ID)
		}
		fail("Não consegui publicar o merge. Tente de novo.")
		return
	}
	if err := h.store.SetMessageID(ctx, m.ID, msg.ChannelID, msg.ID); err != nil {
		slog.Warn("salvando message_id", "err", err, "merge_id", m.ID)
	}
	slog.Info("merge registrado", "merge_id", m.ID, "author", m.AuthorID, "reviewer", m.ReviewerID, "ddl", m.HasDDL)
}

func optionByName(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) *discordgo.ApplicationCommandInteractionDataOption {
	for _, o := range opts {
		if o.Name == name {
			return o
		}
	}
	return nil
}

func (h *Handler) handleComoEstamos(s *discordgo.Session, i *discordgo.InteractionCreate) {
	h.listPending(s, i, store.Filter{
		ReviewerID: i.Member.User.ID,
		AuthorID:   userOption(i, optCriador),
	})
}

func (h *Handler) handlePendentes(s *discordgo.Session, i *discordgo.InteractionCreate) {
	h.listPending(s, i, store.Filter{
		ReviewerID: userOption(i, optResponsavel),
		AuthorID:   userOption(i, optCriador),
	})
}

func userOption(i *discordgo.InteractionCreate, name string) string {
	if opt := i.ApplicationCommandData().GetOption(name); opt != nil {
		return opt.UserValue(nil).ID
	}
	return ""
}

func (h *Handler) listPending(s *discordgo.Session, i *discordgo.InteractionCreate, f store.Filter) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	merges, err := h.store.ListPending(ctx, i.GuildID, f)
	if err != nil {
		slog.Error("listando pendentes", "err", err)
		respondEphemeral(s, i, "Não foi possível listar os merges. Tente de novo.")
		return
	}
	if len(merges) == 0 {
		respondEphemeral(s, i, "Nenhum merge pendente")
		return
	}

	loc := time.UTC
	if gs, err := h.settings(ctx, i.GuildID); err != nil {
		slog.Warn("lendo configuração do servidor", "err", err, "guild_id", i.GuildID)
	} else if l, err := time.LoadLocation(gs.Timezone); err == nil {
		loc = l
	}

	flags := discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2
	for n, components := range pendingPages(merges, loc, pendingHeader(len(merges), f), i.Member.User.ID) {
		if n == 0 {
			err = respondComponents(s, i, discordgo.InteractionResponseChannelMessageWithSource, components, flags)
		} else {
			_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Components:      components,
				Flags:           flags,
				AllowedMentions: &discordgo.MessageAllowedMentions{},
			})
		}
		if err != nil {
			slog.Error("enviando lista de pendentes", "err", err, "page", n)
			return
		}
	}
}

func pendingHeader(total int, f store.Filter) string {
	var b strings.Builder
	if total == 1 {
		b.WriteString("**1 merge pendente**")
	} else {
		fmt.Fprintf(&b, "**%d merges pendentes**", total)
	}
	if f.ReviewerID != "" {
		b.WriteString(" para ")
		b.WriteString(mention(f.ReviewerID))
	}
	if f.AuthorID != "" {
		b.WriteString(" abertos por ")
		b.WriteString(mention(f.AuthorID))
	}
	return b.String()
}

func (h *Handler) handleDoneButton(s *discordgo.Session, i *discordgo.InteractionCreate, id int64) {
	customID := i.MessageComponentData().CustomID

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

	doneLabel := "Aprovado por " + i.Member.DisplayName()
	doneEmbeds := []*discordgo.MessageEmbed{announcementDoneEmbed(m, mention(user.ID))}
	doneComponents := announcementComponents(m, doneLabel)

	if m.MessageID != "" && i.Message.ID == m.MessageID {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds:          doneEmbeds,
				Components:      doneComponents,
				AllowedMentions: &discordgo.MessageAllowedMentions{},
			},
		})
		if err != nil {
			slog.Error("atualizando anúncio", "err", err, "merge_id", id)
		}
		return
	}

	if err := updateClickedMessage(s, i, customID, doneLabel); err != nil {
		slog.Error("atualizando mensagem da lista", "err", err, "merge_id", id)
	}
	h.editAnnouncement(s, m, doneEmbeds[0], doneComponents, nil)
}

func (h *Handler) respondAlreadyDone(s *discordgo.Session, i *discordgo.InteractionCreate, customID string) {
	if err := updateClickedMessage(s, i, customID, "Concluído"); err != nil {
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

func updateClickedMessage(s *discordgo.Session, i *discordgo.InteractionCreate, customID, label string) error {
	return respondComponents(s, i, discordgo.InteractionResponseUpdateMessage,
		disableButton(i.Message.Components, customID, label),
		i.Message.Flags&discordgo.MessageFlagsIsComponentsV2)
}

func respondComponents(s *discordgo.Session, i *discordgo.InteractionCreate, typ discordgo.InteractionResponseType,
	components []discordgo.MessageComponent, flags discordgo.MessageFlags) error {
	type data struct {
		Components      []discordgo.MessageComponent      `json:"components"`
		Flags           discordgo.MessageFlags            `json:"flags,omitempty"`
		AllowedMentions *discordgo.MessageAllowedMentions `json:"allowed_mentions"`
	}
	body := struct {
		Type discordgo.InteractionResponseType `json:"type"`
		Data data                              `json:"data"`
	}{typ, data{components, flags, &discordgo.MessageAllowedMentions{}}}

	endpoint := discordgo.EndpointInteractionResponse(i.ID, i.Token)
	_, err := s.RequestWithBucketID("POST", endpoint, body, endpoint)
	return err
}

func isManager(m *discordgo.Member) bool {
	return m.Permissions&(discordgo.PermissionAdministrator|discordgo.PermissionManageGuild) != 0
}

func parseLinks(rawTasks, rawMerges string) (taskURLs, mergeURLs []string, problem string) {
	taskURLs, err := parseOptionalURLs(rawTasks)
	if err != nil {
		return nil, nil, fmt.Sprintf("Link da tarefa inválido: %v", err)
	}
	mergeURLs, err = parseOptionalURLs(rawMerges)
	if err != nil {
		return nil, nil, fmt.Sprintf("Link do merge inválido: %v", err)
	}
	if len(taskURLs) == 0 && len(mergeURLs) == 0 {
		return nil, nil, "Informe ao menos o link da tarefa ou o do merge."
	}
	return taskURLs, mergeURLs, ""
}

func parseOptionalURLs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	return parseURLs(raw)
}

func parseURLs(raw string) ([]string, error) {
	var urls []string
	for _, field := range strings.Fields(raw) {
		u := strings.TrimRight(field, ",;")
		if u == "" {
			continue
		}
		if err := validateURL(u); err != nil {
			return nil, fmt.Errorf("%s: %w", u, err)
		}
		if !slices.Contains(urls, u) {
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 {
		return nil, errors.New("vazio")
	}
	return urls, nil
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
