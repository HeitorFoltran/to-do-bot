package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"merge-bot/internal/store"
)

const (
	inputTarefa = "tarefa"
	inputMerge  = "merge"
	inputObs    = "obs"
	inputDDL    = "ddl"
)

func (h *Handler) loadForOwner(s *discordgo.Session, i *discordgo.InteractionCreate, id int64, verb string) (*store.Merge, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	m, err := h.store.Get(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && m.GuildID != i.GuildID) {
		respondEphemeral(s, i, "Esse merge já foi aprovado ou cancelado.")
		return nil, false
	}
	if err != nil {
		slog.Error("buscando merge", "err", err, "merge_id", id)
		respondEphemeral(s, i, "Não foi possível carregar o merge. Tente de novo.")
		return nil, false
	}
	if i.Member.User.ID != m.AuthorID && !isManager(i.Member) {
		respondEphemeral(s, i, fmt.Sprintf("Só quem criou o merge (%s) ou um administrador pode %s.", mention(m.AuthorID), verb))
		return nil, false
	}
	return m, true
}

func editPanel(m *store.Merge) []discordgo.MessageComponent {
	one := 1
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				MenuType:    discordgo.UserSelectMenu,
				CustomID:    actionID(actEditReviewer, m.ID),
				Placeholder: "Responsável",
				MinValues:   &one,
				MaxValues:   1,
				DefaultValues: []discordgo.SelectMenuDefaultValue{
					{ID: m.ReviewerID, Type: discordgo.SelectMenuDefaultValueUser},
				},
			},
		}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Editar links e obs",
				Style:    discordgo.PrimaryButton,
				CustomID: actionID(actEditLinks, m.ID),
			},
		}},
	}
}

func (h *Handler) handleEditButton(s *discordgo.Session, i *discordgo.InteractionCreate, id int64) {
	m, ok := h.loadForOwner(s, i, id, "editar")
	if !ok {
		return
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:         fmt.Sprintf("Editando o merge #%d. Escolha o novo responsável ou edite os links e a obs.", m.ID),
			Components:      editPanel(m),
			Flags:           discordgo.MessageFlagsEphemeral,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	})
	if err != nil {
		slog.Error("abrindo painel de edição", "err", err, "merge_id", id)
	}
}

func (h *Handler) handleEditLinksButton(s *discordgo.Session, i *discordgo.InteractionCreate, id int64) {
	m, ok := h.loadForOwner(s, i, id, "editar")
	if !ok {
		return
	}
	err := s.InteractionRespond(i.Interaction, linksModal(actionID(actEditModal, m.ID), fmt.Sprintf("Editar merge #%d", m.ID),
		"DDL nova (vazio mantém a atual)", m.TaskURLs, m.MergeURLs, m.Note))
	if err != nil {
		slog.Error("abrindo modal de edição", "err", err, "merge_id", id)
	}
}

func linksModal(customID, title, ddlLabel string, taskURLs, mergeURLs []string, note string) *discordgo.InteractionResponse {
	textInput := func(customID, label, value string, maxLen int) discordgo.MessageComponent {
		return discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:  customID,
				Label:     label,
				Style:     discordgo.TextInputParagraph,
				Value:     value,
				MaxLength: maxLen,
			},
		}}
	}
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: customID,
			Title:    title,
			Components: []discordgo.MessageComponent{
				textInput(inputTarefa, "Tarefa(s), um link por linha", strings.Join(taskURLs, "\n"), maxURLLen),
				textInput(inputMerge, "Merge(s), um link por linha", strings.Join(mergeURLs, "\n"), maxURLLen),
				textInput(inputObs, "Obs", note, maxNoteLen),
				textInput(inputDDL, ddlLabel, "", maxDDLLen),
			},
		},
	}
}

type linksForm struct {
	taskURLs, mergeURLs []string
	note, ddl           string
}

func parseLinksModal(i *discordgo.InteractionCreate) (linksForm, string) {
	values := modalValues(i.ModalSubmitData().Components)
	taskURLs, mergeURLs, problem := parseLinks(values[inputTarefa], values[inputMerge])
	if problem != "" {
		return linksForm{}, problem
	}
	note := strings.TrimSpace(values[inputObs])
	if utf8.RuneCountInString(note) > maxNoteLen {
		return linksForm{}, fmt.Sprintf("A observação pode ter no máximo %d caracteres.", maxNoteLen)
	}
	ddl := strings.TrimSpace(values[inputDDL])
	if utf8.RuneCountInString(ddl) > maxDDLLen {
		return linksForm{}, fmt.Sprintf("A DDL colada pode ter no máximo %d caracteres. Para mais, anexe um arquivo no /merge.", maxDDLLen)
	}
	return linksForm{taskURLs: taskURLs, mergeURLs: mergeURLs, note: note, ddl: ddl}, ""
}

func (h *Handler) handleEditModal(s *discordgo.Session, i *discordgo.InteractionCreate, id int64) {
	m, ok := h.loadForOwner(s, i, id, "editar")
	if !ok {
		return
	}
	form, msg := parseLinksModal(i)
	if msg != "" {
		respondEphemeral(s, i, msg)
		return
	}
	if form.ddl != "" && m.MessageID == "" {
		respondEphemeral(s, i, "Não encontrei o anúncio deste merge para anexar a DDL.")
		return
	}

	hadDDL := m.HasDDL
	m.TaskURLs, m.MergeURLs, m.Note = form.taskURLs, form.mergeURLs, form.note
	m.HasDDL = hadDDL || form.ddl != ""
	if !h.saveEdit(s, i, m) {
		return
	}
	if form.ddl == "" {
		h.updatePanel(s, i, fmt.Sprintf("Links e obs do merge #%d atualizados.", m.ID), editPanel(m))
		h.refreshAnnouncement(s, m)
		return
	}

	content := fmt.Sprintf("Links, obs e DDL do merge #%d atualizados.", m.ID)
	files := []*discordgo.File{ddlFile(m.ID, form.ddl)}
	if err := h.editAnnouncement(s, m, announcementEmbed(m), announcementComponents(m, ""), files); err != nil {
		content = fmt.Sprintf("Links e obs do merge #%d atualizados, mas não consegui trocar a DDL no anúncio. Tente de novo.", m.ID)
		if !hadDDL {
			m.HasDDL = false
			h.saveEditQuiet(m)
			h.refreshAnnouncement(s, m)
		}
	}
	h.updatePanel(s, i, content, editPanel(m))
}

func (h *Handler) handleEditReviewer(s *discordgo.Session, i *discordgo.InteractionCreate, id int64) {
	m, ok := h.loadForOwner(s, i, id, "editar")
	if !ok {
		return
	}
	data := i.MessageComponentData()
	if len(data.Values) == 0 {
		respondEphemeral(s, i, "Escolha um responsável.")
		return
	}
	reviewerID := data.Values[0]
	if u := data.Resolved.Users[reviewerID]; u != nil && u.Bot {
		respondEphemeral(s, i, "O responsável não pode ser um bot.")
		return
	}

	m.ReviewerID = reviewerID
	if !h.saveEdit(s, i, m) {
		return
	}
	h.updatePanel(s, i, fmt.Sprintf("Responsável do merge #%d agora é %s.", m.ID, mention(reviewerID)), editPanel(m))
	h.refreshAnnouncement(s, m)
}

func (h *Handler) saveEdit(s *discordgo.Session, i *discordgo.InteractionCreate, m *store.Merge) bool {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	err := h.store.Update(ctx, m)
	if errors.Is(err, store.ErrNotFound) {
		respondEphemeral(s, i, "Esse merge já foi aprovado ou cancelado.")
		return false
	}
	if err != nil {
		slog.Error("editando merge", "err", err, "merge_id", m.ID)
		respondEphemeral(s, i, "Não foi possível salvar. Tente de novo.")
		return false
	}
	slog.Info("merge editado", "merge_id", m.ID, "by", i.Member.User.ID)
	return true
}

func (h *Handler) saveEditQuiet(m *store.Merge) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if err := h.store.Update(ctx, m); err != nil {
		slog.Error("desfazendo has_ddl", "err", err, "merge_id", m.ID)
	}
}

func (h *Handler) handleCancelButton(s *discordgo.Session, i *discordgo.InteractionCreate, id int64) {
	m, ok := h.loadForOwner(s, i, id, "cancelar")
	if !ok {
		return
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Cancelar o merge #%d? O registro será excluído e isso não pode ser desfeito.", m.ID),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "Sim, cancelar",
						Style:    discordgo.DangerButton,
						CustomID: actionID(actCancelConfirm, m.ID),
					},
				}},
			},
			Flags:           discordgo.MessageFlagsEphemeral,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	})
	if err != nil {
		slog.Error("pedindo confirmação de cancelamento", "err", err, "merge_id", id)
	}
}

func (h *Handler) handleCancelConfirm(s *discordgo.Session, i *discordgo.InteractionCreate, id int64) {
	m, ok := h.loadForOwner(s, i, id, "cancelar")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if err := h.store.Delete(ctx, id); errors.Is(err, store.ErrNotFound) {
		h.updatePanel(s, i, "Esse merge já foi aprovado ou cancelado.", nil)
		return
	} else if err != nil {
		slog.Error("cancelando merge", "err", err, "merge_id", id)
		respondEphemeral(s, i, "Não foi possível cancelar. Tente de novo.")
		return
	}
	slog.Info("merge cancelado", "merge_id", id, "by", i.Member.User.ID)

	h.updatePanel(s, i, fmt.Sprintf("Merge #%d cancelado.", m.ID), nil)
	h.editAnnouncement(s, m,
		announcementCancelledEmbed(m, mention(i.Member.User.ID)),
		announcementComponents(m, "Cancelado por "+i.Member.DisplayName()), nil)
}

func (h *Handler) updatePanel(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	if components == nil {
		components = []discordgo.MessageComponent{}
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:         content,
			Components:      components,
			AllowedMentions: &discordgo.MessageAllowedMentions{},
		},
	})
	if err != nil {
		slog.Error("atualizando painel", "err", err)
	}
}

func (h *Handler) refreshAnnouncement(s *discordgo.Session, m *store.Merge) {
	h.editAnnouncement(s, m, announcementEmbed(m), announcementComponents(m, ""), nil)
}

// editAnnouncement troca os anexos do anúncio por files quando files não é nil.
func (h *Handler) editAnnouncement(s *discordgo.Session, m *store.Merge, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, files []*discordgo.File) error {
	if m.MessageID == "" {
		return nil
	}
	edit := discordgo.NewMessageEdit(m.ChannelID, m.MessageID).SetEmbeds([]*discordgo.MessageEmbed{embed})
	edit.Components = &components
	edit.AllowedMentions = &discordgo.MessageAllowedMentions{}
	if files != nil {
		edit.Files = files
		edit.Attachments = &[]*discordgo.MessageAttachment{}
	}
	if _, err := s.ChannelMessageEditComplex(edit); err != nil {
		slog.Warn("editando anúncio", "err", err, "merge_id", m.ID)
		return err
	}
	return nil
}

func modalValues(components []discordgo.MessageComponent) map[string]string {
	values := map[string]string{}
	for _, c := range components {
		var row discordgo.ActionsRow
		switch r := c.(type) {
		case *discordgo.ActionsRow:
			row = *r
		case discordgo.ActionsRow:
			row = r
		default:
			continue
		}
		for _, ic := range row.Components {
			switch t := ic.(type) {
			case *discordgo.TextInput:
				values[t.CustomID] = t.Value
			case discordgo.TextInput:
				values[t.CustomID] = t.Value
			}
		}
	}
	return values
}
