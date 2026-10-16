package reminder

import (
	"strings"
	"testing"
	"time"

	"merge-bot/internal/store"
)

func TestGroupByReviewer(t *testing.T) {
	merges := []store.Merge{
		{ID: 1, ReviewerID: "r1"},
		{ID: 2, ReviewerID: "r2"},
		{ID: 3, ReviewerID: "r1"},
	}
	reviewers, by := groupByReviewer(merges)
	if strings.Join(reviewers, ",") != "r1,r2" {
		t.Errorf("reviewers = %v", reviewers)
	}
	if len(by["r1"]) != 2 || by["r1"][0].ID != 1 || by["r1"][1].ID != 3 || len(by["r2"]) != 1 {
		t.Errorf("agrupamento errado: %+v", by)
	}
}

func TestDMText(t *testing.T) {
	if got := dmText(1); got != "**Você tem 1 merge pendente de aprovação.**" {
		t.Errorf("dmText(1) = %q", got)
	}
	if got := dmText(10); got != "**Você tem 10 merges pendentes de aprovação.**" {
		t.Errorf("dmText(10) = %q", got)
	}
}

func TestParseSchedule(t *testing.T) {
	valid := []string{"0 9 * * 3", "30 8 * * 1-5", "@weekly", "0 9,15 * * *"}
	for _, expr := range valid {
		if _, err := ParseSchedule(expr, "America/Sao_Paulo"); err != nil {
			t.Errorf("ParseSchedule(%q) = %v", expr, err)
		}
	}

	invalid := []struct{ expr, tz string }{
		{"0 9 * * 3", "Marte/Base"},
		{"0 9 * * 3", ""},
		{"toda quarta", "UTC"},
		{"* * * * *", "UTC"},
		{"0,30 9 * * 3", "UTC"},
		{"@every 10m", "UTC"},
		{"CRON_TZ=UTC 0 9 * * 3", "UTC"},
		{"0 9 30 2 *", "UTC"},
	}
	for _, c := range invalid {
		if _, err := ParseSchedule(c.expr, c.tz); err == nil {
			t.Errorf("ParseSchedule(%q, %q) aceitou", c.expr, c.tz)
		}
	}
}

func TestScheduleUsesTimezone(t *testing.T) {
	sched, err := ParseSchedule("0 9 * * 3", "America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	next := sched.Next(time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)).In(loc)
	if next.Weekday() != time.Wednesday || next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("próximo envio = %v, quer quarta 09:00 em São Paulo", next)
	}
}

func TestSchedulerPerGuild(t *testing.T) {
	sched := NewScheduler(t.Context(), nil)
	defer sched.Stop()

	if err := sched.Set(store.Settings{GuildID: "g1", ReminderCron: "0 9 * * 3", Timezone: "America/Sao_Paulo", ReminderEnabled: true}); err != nil {
		t.Fatalf("Set g1: %v", err)
	}
	if err := sched.Set(store.Settings{GuildID: "g2", ReminderCron: "0 18 * * 5", Timezone: "UTC", ReminderEnabled: true}); err != nil {
		t.Fatalf("Set g2: %v", err)
	}

	n1, ok := sched.Next("g1")
	if !ok || n1.In(mustLoc(t, "America/Sao_Paulo")).Hour() != 9 {
		t.Errorf("Next g1 = %v, %v", n1, ok)
	}
	n2, ok := sched.Next("g2")
	if !ok || n2.UTC().Weekday() != time.Friday || n2.UTC().Hour() != 18 {
		t.Errorf("Next g2 = %v, %v", n2, ok)
	}

	if err := sched.Set(store.Settings{GuildID: "g1", ReminderCron: "0 9 * * 3", Timezone: "UTC", ReminderEnabled: false}); err != nil {
		t.Fatalf("Set g1 off: %v", err)
	}
	if _, ok := sched.Next("g1"); ok {
		t.Error("g1 desligado ainda tem próximo envio")
	}
	if _, ok := sched.Next("g2"); !ok {
		t.Error("g2 perdeu o agendamento")
	}

	sched.Remove("g2")
	if _, ok := sched.Next("g2"); ok {
		t.Error("g2 removido ainda tem próximo envio")
	}

	if err := sched.Set(store.Settings{GuildID: "g3", ReminderCron: "* * * * *", Timezone: "UTC", ReminderEnabled: true}); err == nil {
		t.Error("Set aceitou cron a cada minuto")
	}
}

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestMessageFor(t *testing.T) {
	if got := messageFor(3, ""); got != dmText(3) {
		t.Errorf("sem mensagem = %q", got)
	}
	if got := messageFor(3, "Bom dia! {quantidade} merges esperando, {quantidade} mesmo."); got != "Bom dia! 3 merges esperando, 3 mesmo." {
		t.Errorf("com {quantidade} = %q", got)
	}
	if got := messageFor(0, "Teste"); got != "Teste" {
		t.Errorf("sem placeholder = %q", got)
	}
}
