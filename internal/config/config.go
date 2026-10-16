package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"merge-bot/internal/reminder"
	"merge-bot/internal/store"
)

const (
	DefaultDBPath       = "./merges.db"
	DefaultReminderCron = "0 9 * * 3"
	DefaultTimezone     = "America/Sao_Paulo"

	ReminderOff = "off"
)

type Config struct {
	DiscordToken string
	DBPath       string

	ReminderCron    string
	ReminderEnabled bool
	Timezone        string
}

func (c *Config) DefaultSettings() store.Settings {
	return store.Settings{
		ReminderCron:    c.ReminderCron,
		Timezone:        c.Timezone,
		ReminderEnabled: c.ReminderEnabled,
	}
}

func Load() (*Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (*Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	cfg := &Config{
		DiscordToken:    get("DISCORD_TOKEN", ""),
		DBPath:          get("DB_PATH", DefaultDBPath),
		ReminderCron:    get("REMINDER_CRON", DefaultReminderCron),
		ReminderEnabled: true,
		Timezone:        get("TZ_NAME", DefaultTimezone),
	}

	var errs []error
	if cfg.DiscordToken == "" {
		errs = append(errs, errors.New("DISCORD_TOKEN é obrigatória"))
	}

	if strings.EqualFold(cfg.ReminderCron, ReminderOff) {
		cfg.ReminderCron = DefaultReminderCron
		cfg.ReminderEnabled = false
	}
	if _, err := reminder.ParseSchedule(cfg.ReminderCron, cfg.Timezone); err != nil {
		errs = append(errs, fmt.Errorf("REMINDER_CRON/TZ_NAME inválidos: %w", err))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}
