package bot

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"merge-bot/internal/store"
)

func TestAge(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		ago  time.Duration
		want string
	}{
		{0, "há menos de 1 minuto"},
		{59 * time.Second, "há menos de 1 minuto"},
		{time.Minute, "há 1 minuto"},
		{90 * time.Second, "há 1 minuto"},
		{45 * time.Minute, "há 45 minutos"},
		{time.Hour, "há 1 hora"},
		{3*time.Hour + 59*time.Minute, "há 3 horas"},
		{23 * time.Hour, "há 23 horas"},
		{24 * time.Hour, "há 1 dia"},
		{47 * time.Hour, "há 1 dia"},
		{48 * time.Hour, "há 2 dias"},
		{30 * 24 * time.Hour, "há 30 dias"},
		{-time.Hour, "há menos de 1 minuto"},
	}
	for _, tt := range tests {
		if got := Age(now.Add(-tt.ago), now); got != tt.want {
			t.Errorf("Age(-%v) = %q, quer %q", tt.ago, got, tt.want)
		}
	}
}

func TestActionID(t *testing.T) {
	for _, act := range []string{actDone, actEdit, actEditLinks, actEditModal, actEditReviewer, actCancel, actCancelConfirm} {
		action, id, ok := parseActionID(actionID(act, 42))
		if !ok || action != act || id != 42 {
			t.Errorf("round trip %s = %q, %d, %v", act, action, id, ok)
		}
	}
	for _, bad := range []string{"merge_done:", "merge_done:abc", "merge_done:-1", "merge_done", ""} {
		if _, _, ok := parseActionID(bad); ok {
			t.Errorf("parseActionID(%q) aceitou", bad)
		}
	}
}

func TestDisableButtonActionsRow(t *testing.T) {
	components := []discordgo.MessageComponent{
		&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.Button{Label: "Aprovar", CustomID: "merge_done:1", Style: discordgo.SuccessButton},
		}},
		&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.Button{Label: "Aprovar", CustomID: "merge_done:2", Style: discordgo.SuccessButton},
		}},
	}

	out := disableButton(components, "merge_done:2", "Aprovado por Ana")

	first := out[0].(discordgo.ActionsRow).Components[0].(discordgo.Button)
	if first.Disabled || first.Label != "Aprovar" {
		t.Errorf("botão 1 mudou: %+v", first)
	}
	second := out[1].(discordgo.ActionsRow).Components[0].(discordgo.Button)
	if !second.Disabled || second.Label != "Aprovado por Ana" {
		t.Errorf("botão 2 não foi desabilitado: %+v", second)
	}
	if components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.Button).Disabled {
		t.Error("disableButton alterou os componentes originais")
	}
}

func TestDisableButtonContainers(t *testing.T) {
	m1 := &store.Merge{ID: 1}
	m2 := &store.Merge{ID: 2}
	components := []discordgo.MessageComponent{
		discordgo.TextDisplay{Content: "cabeçalho"},
		pendingItem(m1, "item 1", true),
		pendingItem(m2, "item 2", true),
	}

	raw, err := json.Marshal(map[string]any{"components": components})
	if err != nil {
		t.Fatal(err)
	}
	var msg discordgo.Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}

	out := disableButton(msg.Components, actionID(actDone, 2), "Aprovado por Ana")
	if len(out) != 3 {
		t.Fatalf("len = %d", len(out))
	}

	buttons := func(c discordgo.Container) []discordgo.MessageComponent {
		return c.Components[1].(discordgo.ActionsRow).Components
	}
	item := func(k int) (discordgo.Container, discordgo.Button) {
		c := out[k].(discordgo.Container)
		row := buttons(c)
		return c, row[len(row)-1].(discordgo.Button)
	}
	c1, b1 := item(1)
	if b1.Disabled || b1.Label != "Aprovar" || *c1.AccentColor != colorPending {
		t.Errorf("item 1 mudou: %+v / cor %x", b1, *c1.AccentColor)
	}
	c2, b2 := item(2)
	if !b2.Disabled || b2.Label != "Aprovado por Ana" || *c2.AccentColor != colorDone {
		t.Errorf("item 2 não foi marcado: %+v / cor %x", b2, *c2.AccentColor)
	}

	for _, b := range buttons(c1) {
		if b.(discordgo.Button).Disabled {
			t.Errorf("item 1: %+v desabilitado", b)
		}
	}
	for _, b := range buttons(c2) {
		if !b.(discordgo.Button).Disabled {
			t.Errorf("item 2: %+v deveria estar desabilitado", b)
		}
	}
}

func TestPendingPagesOwnerActions(t *testing.T) {
	merges := []store.Merge{
		{ID: 1, AuthorID: "eu", TaskURLs: []string{"https://t"}, MergeURLs: []string{"https://m"}},
		{ID: 2, AuthorID: "outro", TaskURLs: []string{"https://t"}, MergeURLs: []string{"https://m"}},
	}
	pages := pendingPages(merges, time.UTC, "cabeçalho", "eu")
	if len(pages) != 1 || len(pages[0]) != 3 {
		t.Fatalf("pages = %+v", pages)
	}
	labels := func(c discordgo.MessageComponent) []string {
		var out []string
		for _, b := range c.(discordgo.Container).Components[1].(discordgo.ActionsRow).Components {
			out = append(out, b.(discordgo.Button).Label)
		}
		return out
	}
	if got := labels(pages[0][1]); !reflect.DeepEqual(got, []string{"Editar", "Cancelar", "Aprovar"}) {
		t.Errorf("botões do próprio autor = %v", got)
	}
	if got := labels(pages[0][2]); !reflect.DeepEqual(got, []string{"Aprovar"}) {
		t.Errorf("botões de outro autor = %v", got)
	}
}

func TestModalValues(t *testing.T) {
	raw := `{"custom_id":"merge_edit_modal:7","components":[
		{"type":1,"components":[{"type":4,"custom_id":"tarefa","value":"https://t/1\nhttps://t/2"}]},
		{"type":1,"components":[{"type":4,"custom_id":"obs","value":"nota"}]}]}`
	var data discordgo.ModalSubmitInteractionData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	got := modalValues(data.Components)
	if got["tarefa"] != "https://t/1\nhttps://t/2" || got["obs"] != "nota" {
		t.Errorf("modalValues = %v", got)
	}
}

func TestPaginate(t *testing.T) {
	tests := []struct {
		lengths []int
		want    [][2]int
	}{
		{nil, nil},
		{[]int{10, 10, 10, 10, 10, 10, 10}, [][2]int{{0, 5}, {5, 7}}},
		{[]int{10, 10, 10, 10, 10}, [][2]int{{0, 5}}},
		{[]int{60, 50, 10}, [][2]int{{0, 1}, {1, 3}}},
		{[]int{150, 10}, [][2]int{{0, 1}, {1, 2}}},
	}
	for _, tt := range tests {
		got := paginate(tt.lengths, 5, 100)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("paginate(%v) = %v, quer %v", tt.lengths, got, tt.want)
		}
	}
}

func TestParseURLs(t *testing.T) {
	got, err := parseURLs("  https://a.com/1, https://b.com/2\nhttps://a.com/1;  http://c.com/x?ids=1,2 ")
	if err != nil {
		t.Fatalf("parseURLs: %v", err)
	}
	want := []string{"https://a.com/1", "https://b.com/2", "http://c.com/x?ids=1,2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseURLs = %v, quer %v", got, want)
	}
	for _, bad := range []string{"", "   ", "https://ok.com nao-e-link", ",,"} {
		if _, err := parseURLs(bad); err == nil {
			t.Errorf("parseURLs(%q) aceitou", bad)
		}
	}
}

func TestBulletList(t *testing.T) {
	if got := bulletList([]string{"a", "b"}); got != "• a\n\n• b" {
		t.Errorf("bulletList = %q", got)
	}
	if got := bulletList(nil); got != "Não informado" {
		t.Errorf("bulletList vazio = %q", got)
	}
}

func TestValidateURL(t *testing.T) {
	valid := []string{"https://gitlab.com/x/-/merge_requests/1", "http://jira.local/browse/ABC-1"}
	for _, u := range valid {
		if err := validateURL(u); err != nil {
			t.Errorf("validateURL(%q) = %v", u, err)
		}
	}
	invalid := []string{"", "gitlab.com/x", "ftp://host/file", "https://", "não é link", "javascript:alert(1)"}
	for _, u := range invalid {
		if err := validateURL(u); err == nil {
			t.Errorf("validateURL(%q) aceitou", u)
		}
	}
}

func TestIsManager(t *testing.T) {
	cases := map[int64]bool{
		0:                                   false,
		discordgo.PermissionSendMessages:    false,
		discordgo.PermissionAdministrator:   true,
		discordgo.PermissionManageGuild:     true,
		discordgo.PermissionManageGuild | 1: true,
	}
	for perms, want := range cases {
		if got := isManager(&discordgo.Member{Permissions: perms}); got != want {
			t.Errorf("isManager(%d) = %v, quer %v", perms, got, want)
		}
	}
}

func TestParseLinks(t *testing.T) {
	tasks, merges, msg := parseLinks("", "https://m/1")
	if msg != "" || tasks != nil || len(merges) != 1 {
		t.Errorf("só merge: %v %v %q", tasks, merges, msg)
	}
	tasks, merges, msg = parseLinks("https://t/1", "  ")
	if msg != "" || len(tasks) != 1 || merges != nil {
		t.Errorf("só tarefa: %v %v %q", tasks, merges, msg)
	}
	if _, _, msg := parseLinks("", ""); msg == "" {
		t.Error("aceitou os dois vazios")
	}
	if _, _, msg := parseLinks("nao-e-link", "https://m/1"); msg == "" {
		t.Error("aceitou tarefa inválida")
	}
}

func TestAnnouncementButtonOrder(t *testing.T) {
	row := announcementComponents(&store.Merge{ID: 1}, "")[0].(discordgo.ActionsRow)
	var got []string
	for _, b := range row.Components {
		got = append(got, b.(discordgo.Button).Label)
	}
	if !reflect.DeepEqual(got, []string{"Editar", "Cancelar", "Aprovar"}) {
		t.Errorf("ordem = %v", got)
	}
}

func TestHelpText(t *testing.T) {
	got := helpText()
	for _, want := range []string{
		"`/merge responsavel [ddl]`",
		"`/pendentes [responsavel] [criador]`",
		"`/config lembrete [cron] [fuso] [ativo]`",
		"*(admin)*",
		"`/help`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("helpText sem %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`/config`") {
		t.Errorf("helpText listou /config sem subcomando:\n%s", got)
	}
}
