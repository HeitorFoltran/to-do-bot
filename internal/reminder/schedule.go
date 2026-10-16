package reminder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"merge-bot/internal/store"
)

const MinInterval = time.Hour

func ParseSchedule(expr, tz string) (cron.Schedule, error) {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "TZ=") || strings.HasPrefix(expr, "CRON_TZ=") {
		return nil, errors.New("informe o fuso na opção fuso, não na expressão")
	}
	if _, err := time.LoadLocation(tz); err != nil || tz == "" {
		return nil, fmt.Errorf("fuso inválido: %q", tz)
	}
	sched, err := cron.ParseStandard("CRON_TZ=" + tz + " " + expr)
	if err != nil {
		return nil, fmt.Errorf("expressão cron inválida: %w", err)
	}

	prev := sched.Next(time.Now())
	if prev.IsZero() {
		return nil, errors.New("a expressão cron nunca dispara")
	}
	for range 100 {
		next := sched.Next(prev)
		if next.IsZero() {
			break
		}
		if next.Sub(prev) < MinInterval {
			return nil, fmt.Errorf("o lembrete não pode disparar com menos de %s de intervalo", MinInterval)
		}
		prev = next
	}
	return sched, nil
}

func LoadSettings(ctx context.Context, st *store.Store, guildID string, defaults store.Settings) (store.Settings, error) {
	s, err := st.GetSettings(ctx, guildID)
	if errors.Is(err, store.ErrNotFound) {
		defaults.GuildID = guildID
		return defaults, nil
	}
	if err != nil {
		return store.Settings{}, err
	}
	return *s, nil
}

type Scheduler struct {
	cron    *cron.Cron
	rem     *Reminder
	ctx     context.Context
	mu      sync.Mutex
	entries map[string]cron.EntryID
}

func NewScheduler(ctx context.Context, rem *Reminder) *Scheduler {
	c := cron.New()
	c.Start()
	return &Scheduler{cron: c, rem: rem, ctx: ctx, entries: map[string]cron.EntryID{}}
}

func (s *Scheduler) Set(settings store.Settings) error {
	var sched cron.Schedule
	if settings.ReminderEnabled {
		var err error
		if sched, err = ParseSchedule(settings.ReminderCron, settings.Timezone); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	guildID := settings.GuildID
	if id, ok := s.entries[guildID]; ok {
		s.cron.Remove(id)
		delete(s.entries, guildID)
	}
	if sched == nil {
		slog.Info("lembrete desligado", "guild_id", guildID)
		return nil
	}

	s.entries[guildID] = s.cron.Schedule(sched, cron.FuncJob(func() {
		ctx, cancel := context.WithTimeout(s.ctx, time.Minute)
		defer cancel()
		if _, err := s.rem.Run(ctx, guildID, Options{}); err != nil {
			slog.Error("lembrete falhou", "err", err, "guild_id", guildID)
		}
	}))
	slog.Info("lembrete agendado", "guild_id", guildID, "cron", settings.ReminderCron, "tz", settings.Timezone)
	return nil
}

func (s *Scheduler) Remove(guildID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entries[guildID]; ok {
		s.cron.Remove(id)
		delete(s.entries, guildID)
	}
}

func (s *Scheduler) Next(guildID string) (time.Time, bool) {
	s.mu.Lock()
	id, ok := s.entries[guildID]
	s.mu.Unlock()
	if !ok {
		return time.Time{}, false
	}
	next := s.cron.Entry(id).Next
	return next, !next.IsZero()
}

func (s *Scheduler) Stop() {
	<-s.cron.Stop().Done()
}
