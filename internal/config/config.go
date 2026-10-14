// Package config lê e valida a configuração a partir de variáveis de ambiente.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	DefaultDBPath       = "./merges.db"
	DefaultReminderCron = "0 9 * * 3"
	DefaultTimezone     = "America/Sao_Paulo"
)

type Config struct {
	DiscordToken      string
	GuildID           string
	DBPath            string
	ReminderChannelID string // vazio = lembrete desligado
	ReminderCron      string
	Location          *time.Location
}

// ReminderEnabled indica se o lembrete semanal deve ser agendado.
func (c *Config) ReminderEnabled() bool {
	return c.ReminderChannelID != ""
}

// Load lê as variáveis de ambiente usando os padrões da especificação.
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
		DiscordToken:      get("DISCORD_TOKEN", ""),
		GuildID:           get("GUILD_ID", ""),
		DBPath:            get("DB_PATH", DefaultDBPath),
		ReminderChannelID: get("REMINDER_CHANNEL_ID", ""),
		ReminderCron:      get("REMINDER_CRON", DefaultReminderCron),
	}

	var errs []error
	if cfg.DiscordToken == "" {
		errs = append(errs, errors.New("DISCORD_TOKEN é obrigatória"))
	}
	if cfg.GuildID == "" {
		errs = append(errs, errors.New("GUILD_ID é obrigatória"))
	}

	tzName := get("TZ_NAME", DefaultTimezone)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		errs = append(errs, fmt.Errorf("TZ_NAME inválido %q: %w", tzName, err))
	}
	cfg.Location = loc

	if _, err := cron.ParseStandard(cfg.ReminderCron); err != nil {
		errs = append(errs, fmt.Errorf("REMINDER_CRON inválido %q: %w", cfg.ReminderCron, err))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}
