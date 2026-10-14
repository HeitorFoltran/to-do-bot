package bot

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

const (
	cmdMerge     = "merge"
	cmdPendentes = "pendentes"

	optTarefa      = "tarefa"
	optMerge       = "merge"
	optResponsavel = "responsavel"
	optObs         = "obs"

	maxNoteLen = 200
	maxURLLen  = 500
)

// Commands são os slash commands do bot.
var Commands = []*discordgo.ApplicationCommand{
	{
		Name:        cmdMerge,
		Description: "Registra um merge request pendente de revisão",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optTarefa,
				Description: "Link da tarefa",
				Required:    true,
				MaxLength:   maxURLLen,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optMerge,
				Description: "Link do merge request",
				Required:    true,
				MaxLength:   maxURLLen,
			},
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        optResponsavel,
				Description: "Quem vai revisar/mergear",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optObs,
				Description: "Observação livre",
				MaxLength:   maxNoteLen,
			},
		},
	},
	{
		Name:        cmdPendentes,
		Description: "Lista os merges pendentes",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        optResponsavel,
				Description: "Filtra por responsável",
			},
		},
	},
}

// RegisterCommands sobrescreve os comandos da guild (idempotente).
func RegisterCommands(s *discordgo.Session, appID, guildID string) error {
	if _, err := s.ApplicationCommandBulkOverwrite(appID, guildID, Commands); err != nil {
		return fmt.Errorf("registrando comandos: %w", err)
	}
	return nil
}
