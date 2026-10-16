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
	colorPending   = 0xF1C40F
	colorDone      = 0x2ECC71
	colorCancelled = 0x95A5A6

	notInformed = "Não informado"

	maxFieldLen = 1024
)

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

func bulletList(urls []string) string {
	if len(urls) == 0 {
		return notInformed
	}
	lines := make([]string, len(urls))
	for k, u := range urls {
		lines[k] = "• " + u
	}
	return strings.Join(lines, "\n\n")
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func firstURL(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

func announcementEmbed(m *store.Merge) *discordgo.MessageEmbed {
	fields := []*discordgo.MessageEmbedField{
		{Name: "Autor", Value: mention(m.AuthorID), Inline: true},
		{Name: "Responsável", Value: mention(m.ReviewerID), Inline: true},
		{Name: "Tarefa(s)", Value: truncate(bulletList(m.TaskURLs), maxFieldLen)},
		{Name: "Merge(s)", Value: truncate(bulletList(m.MergeURLs), maxFieldLen)},
	}
	if m.Note != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Obs", Value: m.Note})
	}
	return &discordgo.MessageEmbed{
		Title:     fmt.Sprintf("Merge pendente #%d", m.ID),
		URL:       firstURL(m.MergeURLs),
		Color:     colorPending,
		Fields:    fields,
		Timestamp: m.CreatedAt.Format(time.RFC3339),
	}
}

func announcementDoneEmbed(m *store.Merge, doneBy string) *discordgo.MessageEmbed {
	e := announcementEmbed(m)
	e.Title = fmt.Sprintf("Merge #%d aprovado", m.ID)
	e.Color = colorDone
	e.Fields = append(e.Fields, &discordgo.MessageEmbedField{Name: "Aprovado por", Value: doneBy})
	return e
}

func announcementCancelledEmbed(m *store.Merge, cancelledBy string) *discordgo.MessageEmbed {
	e := announcementEmbed(m)
	e.Title = fmt.Sprintf("Merge #%d cancelado", m.ID)
	e.Color = colorCancelled
	e.Fields = append(e.Fields, &discordgo.MessageEmbedField{Name: "Cancelado por", Value: cancelledBy})
	return e
}

func announcementComponents(m *store.Merge, closedLabel string) []discordgo.MessageComponent {
	if closedLabel != "" {
		return []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    closedLabel,
					Style:    discordgo.SecondaryButton,
					CustomID: actionID(actDone, m.ID),
					Disabled: true,
				},
			}},
		}
	}
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			editButton(m),
			cancelButton(m),
			approveButton(m),
		}},
	}
}

func approveButton(m *store.Merge) discordgo.Button {
	return discordgo.Button{Label: "Aprovar", Style: discordgo.SuccessButton, CustomID: actionID(actDone, m.ID)}
}

func editButton(m *store.Merge) discordgo.Button {
	return discordgo.Button{Label: "Editar", Style: discordgo.SecondaryButton, CustomID: actionID(actEdit, m.ID)}
}

func cancelButton(m *store.Merge) discordgo.Button {
	return discordgo.Button{Label: "Cancelar", Style: discordgo.DangerButton, CustomID: actionID(actCancel, m.ID)}
}

func pendingText(m *store.Merge, loc *time.Location) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### **ID#**%d\n", m.ID)
	fmt.Fprintf(&b, "**Autor:** %s   **Responsável:** %s\n", mention(m.AuthorID), mention(m.ReviewerID))
	fmt.Fprintf(&b, "\n**Tarefa(s):**\n%s\n", bulletList(m.TaskURLs))
	fmt.Fprintf(&b, "\n**Merge(s):**\n%s\n", bulletList(m.MergeURLs))
	if m.Note != "" {
		fmt.Fprintf(&b, "**Obs:** %s\n", m.Note)
	}
	fmt.Fprintf(&b, "\n**Aberto em:** %s", m.CreatedAt.In(loc).Format("02/01/2006 15:04"))
	return b.String()
}

func pendingItem(m *store.Merge, text string, ownerActions bool) discordgo.Container {
	color := colorPending
	buttons := []discordgo.MessageComponent{approveButton(m)}
	if ownerActions {
		buttons = []discordgo.MessageComponent{editButton(m), cancelButton(m), approveButton(m)}
	}
	return discordgo.Container{
		AccentColor: &color,
		Components: []discordgo.MessageComponent{
			discordgo.TextDisplay{Content: text},
			discordgo.ActionsRow{Components: buttons},
		},
	}
}

const (
	actDone          = "merge_done"
	actEdit          = "merge_edit"
	actEditLinks     = "merge_edit_links"
	actEditModal     = "merge_edit_modal"
	actEditReviewer  = "merge_edit_reviewer"
	actCancel        = "merge_cancel"
	actCancelConfirm = "merge_cancel_confirm"
)

func actionID(action string, id int64) string {
	return action + ":" + strconv.FormatInt(id, 10)
}

func parseActionID(customID string) (action string, id int64, ok bool) {
	action, raw, found := strings.Cut(customID, ":")
	if !found {
		return "", 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return "", 0, false
	}
	return action, id, true
}

func disableButton(components []discordgo.MessageComponent, customID, label string) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, 0, len(components))
	for _, c := range components {
		out = append(out, disableIn(c, customID, label))
	}
	return out
}

func disableIn(c discordgo.MessageComponent, customID, label string) discordgo.MessageComponent {
	switch v := c.(type) {
	case *discordgo.ActionsRow:
		return disableIn(*v, customID, label)
	case discordgo.ActionsRow:
		v.Components = disableButton(v.Components, customID, label)
		return v
	case *discordgo.Container:
		return disableIn(*v, customID, label)
	case discordgo.Container:
		if !hasButton(v, customID) {
			v.Components = disableButton(v.Components, customID, label)
			return v
		}
		color := colorDone
		v.AccentColor = &color
		v.Components = disableAll(disableButton(v.Components, customID, label))
		return v
	case *discordgo.Section:
		return disableIn(*v, customID, label)
	case discordgo.Section:
		if v.Accessory != nil {
			v.Accessory = disableIn(v.Accessory, customID, label)
		}
		return v
	case *discordgo.Button:
		return disableIn(*v, customID, label)
	case discordgo.Button:
		if v.CustomID == customID {
			v.Label = label
			v.Disabled = true
		}
		return v
	default:
		return c
	}
}

func disableAll(components []discordgo.MessageComponent) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, 0, len(components))
	for _, c := range components {
		switch v := c.(type) {
		case *discordgo.ActionsRow:
			c = *v
		case *discordgo.Section:
			c = *v
		case *discordgo.Button:
			c = *v
		}
		switch v := c.(type) {
		case discordgo.ActionsRow:
			v.Components = disableAll(v.Components)
			c = v
		case discordgo.Section:
			if v.Accessory != nil {
				v.Accessory = disableAll([]discordgo.MessageComponent{v.Accessory})[0]
			}
			c = v
		case discordgo.Button:
			v.Disabled = true
			c = v
		}
		out = append(out, c)
	}
	return out
}

func hasButton(c discordgo.MessageComponent, customID string) bool {
	switch v := c.(type) {
	case *discordgo.ActionsRow:
		return hasButton(*v, customID)
	case discordgo.ActionsRow:
		return anyHasButton(v.Components, customID)
	case *discordgo.Container:
		return hasButton(*v, customID)
	case discordgo.Container:
		return anyHasButton(v.Components, customID)
	case *discordgo.Section:
		return hasButton(*v, customID)
	case discordgo.Section:
		return v.Accessory != nil && hasButton(v.Accessory, customID)
	case *discordgo.Button:
		return v.CustomID == customID
	case discordgo.Button:
		return v.CustomID == customID
	default:
		return false
	}
}

func anyHasButton(components []discordgo.MessageComponent, customID string) bool {
	for _, c := range components {
		if hasButton(c, customID) {
			return true
		}
	}
	return false
}

func pendingPages(merges []store.Merge, loc *time.Location, header, viewerID string) [][]discordgo.MessageComponent {
	texts := make([]string, len(merges))
	lengths := make([]int, len(merges))
	for k := range merges {
		texts[k] = pendingText(&merges[k], loc)
		lengths[k] = len([]rune(texts[k]))
	}

	var pages [][]discordgo.MessageComponent
	for n, page := range paginate(lengths, itemsPerMessage, maxPageChars) {
		var components []discordgo.MessageComponent
		if n == 0 && header != "" {
			components = append(components, discordgo.TextDisplay{Content: header})
		}
		for k := page[0]; k < page[1]; k++ {
			components = append(components, pendingItem(&merges[k], texts[k], merges[k].AuthorID == viewerID))
		}
		pages = append(pages, components)
	}
	return pages
}

func paginate(lengths []int, maxItems, maxChars int) [][2]int {
	var pages [][2]int
	start, chars := 0, 0
	for k, n := range lengths {
		if k > start && (k-start == maxItems || chars+n > maxChars) {
			pages = append(pages, [2]int{start, k})
			start, chars = k, 0
		}
		chars += n
	}
	if start < len(lengths) {
		pages = append(pages, [2]int{start, len(lengths)})
	}
	return pages
}
