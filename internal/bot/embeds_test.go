package bot

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
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
		{-time.Hour, "há menos de 1 minuto"}, // relógio adiantado
	}
	for _, tt := range tests {
		if got := Age(now.Add(-tt.ago), now); got != tt.want {
			t.Errorf("Age(-%v) = %q, quer %q", tt.ago, got, tt.want)
		}
	}
}

func TestDoneButtonID(t *testing.T) {
	id, ok := parseDoneButtonID(doneButtonID(42))
	if !ok || id != 42 {
		t.Fatalf("round trip = %d, %v", id, ok)
	}
	for _, bad := range []string{"merge_done:", "merge_done:abc", "merge_done:-1", "outro:1"} {
		if _, ok := parseDoneButtonID(bad); ok {
			t.Errorf("parseDoneButtonID(%q) aceitou", bad)
		}
	}
}

func TestDisableButton(t *testing.T) {
	// Como vem do Discord: ponteiros.
	components := []discordgo.MessageComponent{
		&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.Button{Label: "Mergeado #1", CustomID: "merge_done:1", Style: discordgo.SuccessButton},
		}},
		&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.Button{Label: "Mergeado #2", CustomID: "merge_done:2", Style: discordgo.SuccessButton},
		}},
	}

	out := disableButton(components, "merge_done:2", "✅ Mergeado por Ana")

	first := out[0].(discordgo.ActionsRow).Components[0].(discordgo.Button)
	if first.Disabled || first.Label != "Mergeado #1" {
		t.Errorf("botão 1 mudou: %+v", first)
	}
	second := out[1].(discordgo.ActionsRow).Components[0].(discordgo.Button)
	if !second.Disabled || second.Label != "✅ Mergeado por Ana" {
		t.Errorf("botão 2 não foi desabilitado: %+v", second)
	}
	// Original intacto.
	if components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.Button).Disabled {
		t.Error("disableButton alterou os componentes originais")
	}
}

func TestChunk(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	got := chunk(items, 5)
	if len(got) != 3 || len(got[0]) != 5 || len(got[1]) != 5 || len(got[2]) != 1 {
		t.Fatalf("chunk = %v", got)
	}
	if got := chunk([]int{}, 5); len(got) != 0 {
		t.Errorf("chunk vazio = %v", got)
	}
	if got := chunk([]int{1, 2, 3, 4, 5}, 5); len(got) != 1 {
		t.Errorf("chunk exato = %v", got)
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
