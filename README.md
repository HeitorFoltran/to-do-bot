# Merge Bot

Bot de Discord para registrar merge requests pendentes e lembrar os responsáveis.

## Criar a aplicação no Discord

1. Em [Discord Developer Portal](https://discord.com/developers/applications), clique em **New Application**.
2. Em **Bot**, clique em **Reset Token** e guarde o token (`DISCORD_TOKEN`).
3. Convide o bot para cada servidor com o link abaixo, trocando `<APP_ID>` pelo Application ID (aba **General Information**):

```
https://discord.com/oauth2/authorize?client_id=<APP_ID>&scope=bot+applications.commands&permissions=19456
```

## Variáveis de ambiente

Copie `.env.example` para `.env` e preencha.

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `DISCORD_TOKEN` | sim | | Token do bot |
| `DB_PATH` | não | `./merges.db` | Arquivo SQLite (no Docker é sempre `/data/merges.db`) |
| `REMINDER_CRON` | não | `0 9 * * 3` | Cron padrão do lembrete para servidores sem `/config`. `off` desliga por padrão. |
| `TZ_NAME` | não | `America/Sao_Paulo` | Fuso padrão para servidores sem `/config` |

## Comandos

| Comando | Uso |
|---|---|
| `/merge responsavel [tarefa] [merge] [obs]` | Registra um merge. Informe ao menos a tarefa ou o merge; vários links separados por espaço. |
| `/pendentes [responsavel] [criador]` | Lista os pendentes (só quem chamou vê). |
| `/como_estamos [criador]` | Lista os pendentes em que você é o responsável. |
| `/config conferir` | Mostra a configuração do lembrete do servidor. |
| `/config lembrete [cron] [fuso] [ativo]` | Altera o lembrete. Intervalo mínimo de 1 hora. |
| `/config testar-lembrete [mensagem] [usuario]` | Envia o lembrete agora. `{quantidade}` na mensagem vira o número de pendentes. |

Botões: **Editar** e **Cancelar** (criador ou admin) e **Aprovar** (responsável ou admin). O `/config` é restrito a quem pode gerenciar o servidor.

## Rodar localmente

Requer Go 1.27+.

```sh
cp .env.example .env
go run ./cmd/bot
go test ./...
```

## Deploy (VPS com Docker)

Em uma VPS Linux com [Docker](https://docs.docker.com/engine/install/) instalado:

```sh
git clone git@github.com:HeitorFoltran/to-do-bot.git
cd to-do-bot
cp .env.example .env   # preencha DISCORD_TOKEN
docker compose up -d --build
docker compose logs -f
```

Não rode o bot em dois lugares com o mesmo token: os comandos seriam respondidos duas vezes.

Atualizar:

```sh
git pull
docker compose up -d --build
```

Alterar o `.env`: edite o arquivo e rode `docker compose up -d`.

Backup do banco (o bot fica parado por alguns segundos, para o arquivo ficar consistente):

```sh
docker compose stop
docker compose cp merge-bot:/data/merges.db ./backup-merges.db
docker compose start
```

Para levar um banco existente para o servidor:

```sh
docker compose stop
docker run --rm -v merge-bot-data:/data -v "$PWD":/src alpine \
  sh -c 'cp /src/merges.db /data/merges.db && rm -f /data/merges.db-wal /data/merges.db-shm && chown 65532:65532 /data/merges.db'
docker compose start
```
