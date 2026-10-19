package bot

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	cmdMerge       = "merge"
	cmdPendentes   = "pendentes"
	cmdComoEstamos = "como_estamos"
	cmdConfig      = "config"
	cmdHelp        = "help"

	subConferir       = "conferir"
	subLembrete       = "lembrete"
	subTestarLembrete = "testar-lembrete"

	optCron  = "cron"
	optFuso  = "fuso"
	optAtivo = "ativo"

	optMensagem = "mensagem"
	optUsuario  = "usuario"

	maxTestMessageLen = 1000

	optResponsavel = "responsavel"
	optCriador     = "criador"
	optDDL         = "ddl"

	maxNoteLen = 200
	maxURLLen  = 800
)

var Commands = []*discordgo.ApplicationCommand{
	{
		Name:        cmdMerge,
		Description: "Registra um merge request pendente de revisão (links, obs e DDL no formulário)",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        optResponsavel,
				Description: "Quem vai revisar/mergear",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        optDDL,
				Description: "Arquivo com a DDL (opcional)",
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

func init() {
	Commands = append(Commands, &discordgo.ApplicationCommand{
		Name:        cmdHelp,
		Description: "Lista os comandos do bot",
	})
}

// helpText monta a ajuda a partir de Commands, para não sair de sincronia.
func helpText() string {
	var b strings.Builder
	b.WriteString("**Comandos**\n")
	for _, c := range Commands {
		admin := ""
		if c.DefaultMemberPermissions != nil {
			admin = " *(admin)*"
		}
		subs := 0
		for _, o := range c.Options {
			if o.Type == discordgo.ApplicationCommandOptionSubCommand {
				subs++
				fmt.Fprintf(&b, "`/%s %s%s` — %s%s\n", c.Name, o.Name, usage(o.Options), o.Description, admin)
			}
		}
		if subs == 0 {
			fmt.Fprintf(&b, "`/%s%s` — %s%s\n", c.Name, usage(c.Options), c.Description, admin)
		}
	}
	b.WriteString("\nOpções entre [colchetes] são opcionais.")
	return b.String()
}

func usage(opts []*discordgo.ApplicationCommandOption) string {
	var b strings.Builder
	for _, o := range opts {
		if o.Required {
			fmt.Fprintf(&b, " %s", o.Name)
		} else {
			fmt.Fprintf(&b, " [%s]", o.Name)
		}
	}
	return b.String()
}

var manageGuild int64 = discordgo.PermissionManageGuild

func RegisterCommands(s *discordgo.Session, appID, guildID string) error {
	if _, err := s.ApplicationCommandBulkOverwrite(appID, guildID, Commands); err != nil {
		return fmt.Errorf("registrando comandos: %w", err)
	}
	return nil
}
