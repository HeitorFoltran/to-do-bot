package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"merge-bot/internal/store"
)

const (
	colorPending = 0xF1C40F // amarelo
	colorDone    = 0x2ECC71 // verde

	doneButtonPrefix = "merge_done:"
)

// Age formata há quanto tempo algo aconteceu: "há 2 dias", "há 3 horas".
func Age(since, now time.Time) string {
	d := now.Sub(since)
	switch {
	case d < time.Minute:
		return "há menos de 1 minuto"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minuto", "minutos")
	case d < 24*time.Hour:
		return plural(int(d/time.Hour), "hora", "horas")
	default:
		return plural(int(d/(24*time.Hour)), "dia", "dias")
	}
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return "há 1 " + singular
	}
	return fmt.Sprintf("há %d %s", n, pluralForm)
}

func mention(userID string) string {
	return "<@" + userID + ">"
}

func baseFields(m *store.Merge) []*discordgo.MessageEmbedField {
	fields := []*discordgo.MessageEmbedField{
		{Name: "Autor", Value: mention(m.AuthorID), Inline: true},
		{Name: "Responsável", Value: mention(m.ReviewerID), Inline: true},
		{Name: "Tarefa", Value: m.TaskURL},
		{Name: "Merge", Value: m.MergeURL},
	}
	if m.Note != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Obs", Value: m.Note})
	}
	return fields
}

// announcementEmbed é o embed postado por /merge.
func announcementEmbed(m *store.Merge) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:     fmt.Sprintf("🔀 Merge pendente #%d", m.ID),
		URL:       m.MergeURL,
		Color:     colorPending,
		Fields:    baseFields(m),
		Timestamp: m.CreatedAt.Format(time.RFC3339),
	}
}

// announcementDoneEmbed é o anúncio original marcado como mergeado.
func announcementDoneEmbed(m *store.Merge, doneBy string) *discordgo.MessageEmbed {
	e := announcementEmbed(m)
	e.Title = fmt.Sprintf("✅ Merge #%d mergeado", m.ID)
	e.Color = colorDone
	e.Fields = append(e.Fields, &discordgo.MessageEmbedField{Name: "Mergeado por", Value: doneBy})
	return e
}

// pendingEmbed é o item da listagem de /pendentes.
func pendingEmbed(m *store.Merge, now time.Time, loc *time.Location) *discordgo.MessageEmbed {
	fields := baseFields(m)
	fields = append(fields, &discordgo.MessageEmbedField{
		Name:  "Aberto",
		Value: fmt.Sprintf("%s (%s)", m.CreatedAt.In(loc).Format("02/01/2006 15:04"), Age(m.CreatedAt, now)),
	})
	return &discordgo.MessageEmbed{
		Title:  fmt.Sprintf("#%d", m.ID),
		URL:    m.MergeURL,
		Color:  colorPending,
		Fields: fields,
	}
}

func doneButtonID(id int64) string {
	return doneButtonPrefix + strconv.FormatInt(id, 10)
}

func parseDoneButtonID(customID string) (int64, bool) {
	raw, ok := strings.CutPrefix(customID, doneButtonPrefix)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func doneButtonRow(m *store.Merge) discordgo.ActionsRow {
	return discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{
			Label:    fmt.Sprintf("Mergeado #%d", m.ID),
			Style:    discordgo.SuccessButton,
			CustomID: doneButtonID(m.ID),
		},
	}}
}

// disableButton devolve uma cópia dos componentes com o botão customID
// desabilitado e com o novo label. Os demais componentes não mudam.
func disableButton(components []discordgo.MessageComponent, customID, label string) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, 0, len(components))
	for _, c := range components {
		var row discordgo.ActionsRow
		switch r := c.(type) {
		case *discordgo.ActionsRow:
			row = *r
		case discordgo.ActionsRow:
			row = r
		default:
			out = append(out, c)
			continue
		}
		inner := make([]discordgo.MessageComponent, 0, len(row.Components))
		for _, ic := range row.Components {
			var btn discordgo.Button
			switch b := ic.(type) {
			case *discordgo.Button:
				btn = *b
			case discordgo.Button:
				btn = b
			default:
				inner = append(inner, ic)
				continue
			}
			if btn.CustomID == customID {
				btn.Label = label
				btn.Disabled = true
			}
			inner = append(inner, btn)
		}
		row.Components = inner
		out = append(out, row)
	}
	return out
}

// chunk divide items em grupos de até size elementos.
func chunk[T any](items []T, size int) [][]T {
	var out [][]T
	for size < len(items) {
		items, out = items[size:], append(out, items[:size])
	}
	if len(items) > 0 {
		out = append(out, items)
	}
	return out
}
