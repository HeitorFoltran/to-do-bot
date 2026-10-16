package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rem := reminder.New(session, st)
	sched := reminder.NewScheduler(ctx, rem)
	defer sched.Stop()

	handler := bot.NewHandler(st, rem, sched, cfg.DefaultSettings())
	session.AddHandler(handler.OnInteraction)
	session.AddHandler(handler.OnGuildCreate)
	session.AddHandler(handler.OnGuildDelete)
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("conectado ao Discord", "user", r.User.Username, "servidores", len(r.Guilds))
	})

	if err := session.Open(); err != nil {
		return fmt.Errorf("abrindo conexão com o Discord: %w", err)
	}
	defer session.Close()

	slog.Info("bot rodando; Ctrl+C para sair")
	<-ctx.Done()
	slog.Info("encerrando")

	sched.Stop()
	return nil
}
