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

func TestLoadBackup(t *testing.T) {
	cfg, err := load(env(map[string]string{"DISCORD_TOKEN": "t"}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.BackupDir != "" || cfg.BackupCron != DefaultBackupCron || cfg.BackupKeep != DefaultBackupKeep {
		t.Errorf("padrões de backup: %+v", cfg)
	}

	cfg, err = load(env(map[string]string{"DISCORD_TOKEN": "t", "BACKUP_DIR": "/b", "BACKUP_KEEP": "7"}))
	if err != nil || cfg.BackupDir != "/b" || cfg.BackupKeep != 7 {
		t.Errorf("backup configurado: %+v, %v", cfg, err)
	}

	for _, c := range []map[string]string{
		{"DISCORD_TOKEN": "t", "BACKUP_KEEP": "0"},
		{"DISCORD_TOKEN": "t", "BACKUP_KEEP": "muitos"},
		{"DISCORD_TOKEN": "t", "BACKUP_DIR": "/b", "BACKUP_CRON": "* * * * *"},
	} {
		if _, err := load(env(c)); err == nil {
			t.Errorf("load(%v) deveria falhar", c)
		}
	}
}
