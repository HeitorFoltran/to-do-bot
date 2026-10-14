package config

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{"DISCORD_TOKEN": "t", "GUILD_ID": "g"}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DBPath != DefaultDBPath || cfg.ReminderCron != DefaultReminderCron ||
		cfg.Location.String() != DefaultTimezone || cfg.ReminderEnabled() {
		t.Errorf("padrões errados: %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []map[string]string{
		{"GUILD_ID": "g"},
		{"DISCORD_TOKEN": "t"},
		{"DISCORD_TOKEN": "t", "GUILD_ID": "g", "TZ_NAME": "Marte/Base"},
		{"DISCORD_TOKEN": "t", "GUILD_ID": "g", "REMINDER_CRON": "toda quarta"},
	}
	for _, c := range cases {
		if _, err := load(env(c)); err == nil {
			t.Errorf("load(%v) deveria falhar", c)
		}
	}
}
