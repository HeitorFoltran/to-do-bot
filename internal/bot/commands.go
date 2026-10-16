package bot

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

const (
	cmdMerge       = "merge"
	cmdPendentes   = "pendentes"
	cmdComoEstamos = "como_estamos"
	cmdConfig      = "config"

	subConferir       = "conferir"
	subLembrete       = "lembrete"
	subTestarLembrete = "testar-lembrete"

	optCron  = "cron"
	optFuso  = "fuso"
	optAtivo = "ativo"

	optMensagem = "mensagem"
	optUsuario  = "usuario"

	maxTestMessageLen = 1000

	optTarefa      = "tarefa"
	optMerge       = "merge"
	optResponsavel = "responsavel"
	optCriador     = "criador"
	optObs         = "obs"

	maxNoteLen = 200
	maxURLLen  = 800
)

var Commands = []*discordgo.ApplicationCommand{
	{
		Name:        cmdMerge,
		Description: "Registra um merge request pendente de revisão",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        optResponsavel,
				Description: "Quem vai revisar/mergear",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optTarefa,
				Description: "Link(s) da tarefa, separados por espaço (opcional se informar o merge)",
				MaxLength:   maxURLLen,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optMerge,
				Description: "Link(s) do merge request, separados por espaço (opcional se informar a tarefa)",
				MaxLength:   maxURLLen,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optObs,
				Description: "Observação",
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
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        optCriador,
				Description: "Filtra por criador do merge",
			},
		},
	},
	{
		Name:        cmdComoEstamos,
		Description: "Listar merges onde eu sou o responsável",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        optCriador,
				Description: "Filtra por criador do merge",
			},
		},
	},
	{
		Name:        cmdConfig,
		Description: "Configurações do bot neste servidor",

		DefaultMemberPermissions: &manageGuild,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subConferir,
				Description: "Mostra as configurações deste servidor",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subLembrete,
				Description: "Altera o lembrete semanal (preencha só o que quer mudar)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optCron,
						Description: "Expressão cron, ex.: 0 9 * * 3 (quarta às 09:00)",
						MaxLength:   100,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optFuso,
						Description: "Fuso horário, ex.: America/Sao_Paulo",
						MaxLength:   100,
					},
					{
						Type:        discordgo.ApplicationCommandOptionBoolean,
						Name:        optAtivo,
						Description: "Liga ou desliga o lembrete",
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subTestarLembrete,
				Description: "Envia agora as DMs do lembrete deste servidor",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optMensagem,
						Description: "Texto da DM no lugar do padrão; {quantidade} vira o número de pendentes",
						MaxLength:   maxTestMessageLen,
					},
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        optUsuario,
						Description: "Envia só para essa pessoa",
					},
				},
			},
		},
	},
}

var manageGuild int64 = discordgo.PermissionManageGuild

func RegisterCommands(s *discordgo.Session, appID, guildID string) error {
	if _, err := s.ApplicationCommandBulkOverwrite(appID, guildID, Commands); err != nil {
		return fmt.Errorf("registrando comandos: %w", err)
	}
	return nil
}
