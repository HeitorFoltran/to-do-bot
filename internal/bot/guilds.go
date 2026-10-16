package bot

import (
	"context"
	"log/slog"

	"github.com/bwmarrin/discordgo"
)

func (h *Handler) OnGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	if g.Unavailable {
		return
	}
	log := slog.With("guild_id", g.ID, "guild", g.Name)

	if _, done := h.registered.Load(g.ID); !done {
		if err := RegisterCommands(s, s.State.User.ID, g.ID); err != nil {
			log.Error("registrando comandos", "err", err)
		} else {
			h.registered.Store(g.ID, true)
			log.Info("comandos registrados")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	gs, err := h.settings(ctx, g.ID)
	if err != nil {
		log.Error("lendo configuração", "err", err)
		return
	}
	if err := h.sched.Set(gs); err != nil {
		log.Error("agendando lembrete", "err", err)
	}
}

func (h *Handler) OnGuildDelete(s *discordgo.Session, g *discordgo.GuildDelete) {
	if g.Unavailable {
		return
	}
	h.sched.Remove(g.ID)
	h.registered.Delete(g.ID)
	slog.Info("bot saiu do servidor", "guild_id", g.ID)
}
