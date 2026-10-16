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

	_ "time/tzdata"

	"merge-bot/internal/backup"
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

	stopBackups, err := startBackups(ctx, cfg, st)
	if err != nil {
		return err
	}
	defer stopBackups()

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
	stopBackups()
	return nil
}

func startBackups(ctx context.Context, cfg *config.Config, st *store.Store) (stop func(), err error) {
	if cfg.BackupDir == "" {
		slog.Info("backup desligado (BACKUP_DIR vazio)")
		return func() {}, nil
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, err
	}
	schedule, err := reminder.ParseSchedule(cfg.BackupCron, cfg.Timezone)
	if err != nil {
		return nil, err
	}

	b := backup.New(st, cfg.BackupDir, cfg.BackupKeep, loc)
	run := func() {
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		path, err := b.Run(runCtx)
		if err != nil {
			slog.Error("backup falhou", "err", err, "dir", cfg.BackupDir)
			return
		}
		slog.Info("backup criado", "arquivo", path)
	}

	run()
	c := cron.New()
	c.Schedule(schedule, cron.FuncJob(run))
	c.Start()
	slog.Info("backup agendado", "cron", cfg.BackupCron, "tz", cfg.Timezone, "dir", cfg.BackupDir, "manter", cfg.BackupKeep)
	return func() { <-c.Stop().Done() }, nil
}
