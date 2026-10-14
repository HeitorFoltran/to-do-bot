package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"

	// Embute a base de fusos: a imagem final não tem /usr/share/zoneinfo.
	_ "time/tzdata"

	"merge-bot/internal/bot"
	"merge-bot/internal/config"
	"merge-bot/internal/reminder"
	"merge-bot/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("encerrando com erro", "err", err)
		os.Exit(1)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("lendo .env: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return fmt.Errorf("criando sessão: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds

	handler := bot.NewHandler(st, cfg.Location)
	session.AddHandler(handler.OnInteraction)
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("conectado ao Discord", "user", r.User.Username)
	})

	if err := session.Open(); err != nil {
		return fmt.Errorf("abrindo conexão com o Discord: %w", err)
	}
	defer session.Close()

	appID, err := applicationID(session)
	if err != nil {
		return err
	}
	if err := bot.RegisterCommands(session, appID, cfg.GuildID); err != nil {
		return err
	}
	slog.Info("comandos registrados", "guild_id", cfg.GuildID)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var scheduler *cron.Cron
	if cfg.ReminderEnabled() {
		rem := reminder.New(session, st, cfg.GuildID, cfg.ReminderChannelID)
		scheduler = cron.New(cron.WithLocation(cfg.Location))
		if _, err := scheduler.AddFunc(cfg.ReminderCron, func() {
			runCtx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			rem.Run(runCtx)
		}); err != nil {
			return fmt.Errorf("agendando lembrete: %w", err)
		}
		scheduler.Start()
		slog.Info("lembrete agendado", "cron", cfg.ReminderCron, "tz", cfg.Location.String(), "channel_id", cfg.ReminderChannelID)
	} else {
		slog.Info("lembrete desligado (REMINDER_CHANNEL_ID vazio)")
	}

	slog.Info("bot rodando; Ctrl+C para sair")
	<-ctx.Done()
	slog.Info("encerrando")

	if scheduler != nil {
		// Espera um lembrete em andamento terminar.
		<-scheduler.Stop().Done()
	}
	return nil
}

func applicationID(s *discordgo.Session) (string, error) {
	if s.State != nil && s.State.User != nil {
		return s.State.User.ID, nil
	}
	u, err := s.User("@me")
	if err != nil {
		return "", fmt.Errorf("obtendo usuário do bot: %w", err)
	}
	return u.ID, nil
}
