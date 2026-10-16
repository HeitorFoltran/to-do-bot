package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"merge-bot/internal/reminder"
	"merge-bot/internal/store"
)

func (h *Handler) handleConfig(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !isManager(i.Member) {
		respondEphemeral(s, i, "Só quem pode gerenciar o servidor usa o /config.")
		return
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		respondEphemeral(s, i, "Subcomando ausente.")
		return
	}
	switch sub := options[0]; sub.Name {
	case subConferir:
		h.handleConfigConferir(s, i)
	case subLembrete:
		h.handleConfigLembrete(s, i, sub)
	case subTestarLembrete:
		h.handleConfigTestarLembrete(s, i, sub)
	default:
		respondEphemeral(s, i, "Subcomando desconhecido.")
	}
}

func (h *Handler) handleConfigConferir(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	gs, err := h.settings(ctx, i.GuildID)
	if err != nil {
		slog.Error("lendo configuração", "err", err, "guild_id", i.GuildID)
		respondEphemeral(s, i, "Não foi possível ler as configurações. Tente de novo.")
		return
	}
	respondEphemeral(s, i, h.describeSettings(gs))
}

func (h *Handler) handleConfigLembrete(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	if len(sub.Options) == 0 {
		respondEphemeral(s, i, "Informe ao menos uma opção: cron, fuso ou ativo.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	gs, err := h.settings(ctx, i.GuildID)
	if err != nil {
		slog.Error("lendo configuração", "err", err, "guild_id", i.GuildID)
		respondEphemeral(s, i, "Não foi possível ler as configurações. Tente de novo.")
		return
	}

	if opt := sub.GetOption(optCron); opt != nil {
		gs.ReminderCron = strings.TrimSpace(opt.StringValue())
	}
	if opt := sub.GetOption(optFuso); opt != nil {
		gs.Timezone = strings.TrimSpace(opt.StringValue())
	}
	if opt := sub.GetOption(optAtivo); opt != nil {
		gs.ReminderEnabled = opt.BoolValue()
	}

	if _, err := reminder.ParseSchedule(gs.ReminderCron, gs.Timezone); err != nil {
		respondEphemeral(s, i, "Configuração inválida: "+err.Error())
		return
	}
	if err := h.store.SaveSettings(ctx, &gs); err != nil {
		slog.Error("salvando configuração", "err", err, "guild_id", i.GuildID)
		respondEphemeral(s, i, "Não foi possível salvar as configurações. Tente de novo.")
		return
	}
	if err := h.sched.Set(gs); err != nil {
		slog.Error("reagendando lembrete", "err", err, "guild_id", i.GuildID)
	}
	slog.Info("configuração alterada", "guild_id", i.GuildID, "by", i.Member.User.ID,
		"cron", gs.ReminderCron, "tz", gs.Timezone, "ativo", gs.ReminderEnabled)

	respondEphemeral(s, i, "Configurações salvas.\n\n"+h.describeSettings(gs))
}

func (h *Handler) handleConfigTestarLembrete(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	var opt reminder.Options
	if o := sub.GetOption(optMensagem); o != nil {
		opt.Message = strings.TrimSpace(o.StringValue())
	}
	if o := sub.GetOption(optUsuario); o != nil {
		opt.UserID = o.UserValue(nil).ID
		if u := resolvedUser(i.ApplicationCommandData(), opt.UserID); u != nil && u.Bot {
			respondEphemeral(s, i, "Não dá para mandar DM para um bot.")
			return
		}
	}

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
	if err != nil {
		slog.Error("respondendo testar-lembrete", "err", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	res, err := h.rem.Run(ctx, i.GuildID, opt)

	var msg string
	switch {
	case err != nil:
		slog.Error("testar-lembrete", "err", err, "guild_id", i.GuildID)
		msg = "Não foi possível enviar o lembrete. Tente de novo."
	case res.Reviewers == 0 && opt.UserID != "":
		msg = fmt.Sprintf("%s não tem merges pendentes; nenhuma DM enviada. Para mandar mesmo assim, informe uma mensagem.", mention(opt.UserID))
	case res.Reviewers == 0:
		msg = "Nenhum merge pendente; nenhuma DM enviada."
	case opt.UserID != "" && res.Failed > 0:
		msg = fmt.Sprintf("Não foi possível mandar DM para %s (ela não aceita DMs de membros do servidor).", mention(opt.UserID))
	case opt.UserID != "":
		msg = fmt.Sprintf("DM enviada para %s.", mention(opt.UserID))
	default:
		msg = fmt.Sprintf("Lembrete enviado para %d de %d responsáveis.", res.Reviewers-res.Failed, res.Reviewers)
		if res.Failed > 0 {
			msg += fmt.Sprintf(" %d não aceitam DMs de membros do servidor.", res.Failed)
		}
	}
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg}); err != nil {
		slog.Error("editando resposta de testar-lembrete", "err", err)
	}
}

func (h *Handler) describeSettings(gs store.Settings) string {
	status := "desligado"
	if gs.ReminderEnabled {
		status = "ligado"
	}
	next := "nenhum"
	if t, ok := h.sched.Next(gs.GuildID); ok {
		if loc, err := time.LoadLocation(gs.Timezone); err == nil {
			t = t.In(loc)
		}
		next = t.Format("02/01/2006 15:04")
	}

	var b strings.Builder
	b.WriteString("**Lembrete**\n")
	fmt.Fprintf(&b, "Status: %s\n", status)
	fmt.Fprintf(&b, "Cron: `%s`\n", gs.ReminderCron)
	fmt.Fprintf(&b, "Fuso: %s\n", gs.Timezone)
	fmt.Fprintf(&b, "Próximo envio: %s", next)
	return b.String()
}
