package config

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{"DISCORD_TOKEN": "t"}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	d := cfg.DefaultSettings()
	if cfg.DBPath != DefaultDBPath || d.ReminderCron != DefaultReminderCron ||
		d.Timezone != DefaultTimezone || !d.ReminderEnabled {
		t.Errorf("padrões errados: %+v / %+v", cfg, d)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []map[string]string{
		{},
		{"DISCORD_TOKEN": "t", "TZ_NAME": "Marte/Base"},
		{"DISCORD_TOKEN": "t", "REMINDER_CRON": "toda quarta"},
		{"DISCORD_TOKEN": "t", "REMINDER_CRON": "* * * * *"},
	}
	for _, c := range cases {
		if _, err := load(env(c)); err == nil {
			t.Errorf("load(%v) deveria falhar", c)
		}
	}
}

func TestLoadReminderOff(t *testing.T) {
	cfg, err := load(env(map[string]string{"DISCORD_TOKEN": "t", "REMINDER_CRON": "OFF"}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	d := cfg.DefaultSettings()
	if d.ReminderEnabled || d.ReminderCron != DefaultReminderCron {
		t.Errorf("REMINDER_CRON=off: %+v", d)
	}
}
