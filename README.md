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
| `TZ_NAME` | não | `America/Sao_Paulo` | Fuso padrão para servidores sem `/config` e fuso do backup |
| `BACKUP_DIR` | não | | Pasta dos backups. Vazio desliga (no Docker é sempre `/backups`, ligada a `./backups`). |
| `BACKUP_CRON` | não | `0 3 * * *` | Quando fazer o backup (padrão: todo dia às 03:00) |
| `BACKUP_KEEP` | não | `14` | Quantos backups manter; os mais antigos são apagados |

## Comandos

| Comando | Uso |
|---|---|
| `/merge responsavel [ddl]` | Abre um formulário para os links da tarefa e do merge (um por linha), a obs e a DDL. Informe ao menos a tarefa ou o merge. A DDL pode ir como arquivo na opção `ddl` (até 8 MB) ou colada no formulário (até 4000 caracteres); ela fica anexada ao anúncio. |
| `/pendentes [responsavel] [criador]` | Lista os pendentes (só quem chamou vê). |
| `/como_estamos [criador]` | Lista os pendentes em que você é o responsável. |
| `/config conferir` | Mostra a configuração do lembrete do servidor. |
| `/config lembrete [cron] [fuso] [ativo]` | Altera o lembrete. Intervalo mínimo de 1 hora. |
| `/config testar-lembrete [mensagem] [usuario]` | Envia o lembrete agora. `{quantidade}` na mensagem vira o número de pendentes. |
| `/help` | Lista os comandos (só quem chamou vê). |

Botões: **Editar** e **Cancelar** (criador ou admin) e **Aprovar** (responsável ou admin). O `/config` é restrito a quem pode gerenciar o servidor.

## Rodar localmente

Requer Go 1.27+.

```sh
cp .env.example .env
go run ./cmd/bot
go test ./...
```

## Deploy (Ubuntu com Docker)

Em um Ubuntu com [Docker](https://docs.docker.com/engine/install/ubuntu/) instalado:

```sh
git clone git@github.com:HeitorFoltran/to-do-bot.git
cd to-do-bot
cp .env.example .env   # preencha DISCORD_TOKEN
mkdir -p backups && sudo chown 65532:65532 backups
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

## Backup

O bot faz um backup ao subir e depois no horário de `BACKUP_CRON`, sem parar, na pasta `backups/` do projeto (`merges-AAAA-MM-DD_HH-MM-SS.db`), mantendo os últimos `BACKUP_KEEP`. Copie essa pasta para fora do PC de vez em quando.

### Restaurar no mesmo PC

A restauração substitui o banco atual: tudo o que foi registrado depois do backup se perde.

1. Escolha o backup:
   ```sh
   ls -l backups/
   ```
2. Pare o bot e guarde o banco atual por segurança:
   ```sh
   docker compose stop
   docker compose cp merge-bot:/data/merges.db ./backups/antes-de-restaurar.db
   ```
3. Copie o backup para o volume, trocando `ARQUIVO.db` pelo nome escolhido:
   ```sh
   docker run --rm -v merge-bot-data:/data -v "$PWD/backups":/b alpine \
     sh -c 'cp /b/ARQUIVO.db /data/merges.db && rm -f /data/merges.db-wal /data/merges.db-shm && chown 65532:65532 /data/merges.db'
   ```
4. Suba e confira:
   ```sh
   docker compose start
   docker compose logs --tail 20
   ```

### Restaurar em outra máquina

1. Faça os passos de [Deploy](#deploy-ubuntu-com-docker) até criar a pasta `backups/`, sem subir o bot.
2. Copie o backup para `backups/`.
3. Crie o volume e pare o bot:
   ```sh
   docker compose up -d --build
   docker compose stop
   ```
4. Siga os passos 3 e 4 de "Restaurar no mesmo PC".
