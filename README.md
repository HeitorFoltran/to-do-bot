# Merge Bot

Bot de Discord que registra merge requests pendentes da equipe e lembra o responsável de revisá-los.

- `/merge tarefa:<link> merge:<link> responsavel:@pessoa [obs:<texto>]` registra o merge e posta um anúncio mencionando o responsável.
- `/pendentes [responsavel:@pessoa]` lista os pendentes, do mais antigo ao mais novo, 5 por mensagem, cada um com o botão **Mergeado #id**.
- O botão só funciona para o responsável ou para quem tem permissão `Administrator` ou `Manage Server`. Ao concluir, o registro é excluído, o botão é desabilitado e o anúncio original fica verde com ✅.
- Toda quarta às 09:00 (configurável), o bot posta no canal de lembrete os pendentes agrupados por responsável. Se não houver pendentes, não posta nada.

## Criando a aplicação no Discord

1. Acesse o [Discord Developer Portal](https://discord.com/developers/applications) e clique em **New Application**.
2. Em **Bot**, clique em **Reset Token** e copie o token para `DISCORD_TOKEN`. Nenhuma *Privileged Gateway Intent* precisa ser ligada.
3. Em **OAuth2 → URL Generator**, marque:
   - Escopos: `bot` e `applications.commands`
   - Permissões do bot: **Send Messages** e **Embed Links** (opcionalmente **View Channels**, caso o cargo padrão não a tenha nos canais usados)
4. Abra a URL gerada e convide o bot para o servidor.
5. Com o modo desenvolvedor do Discord ligado (Configurações → Avançado), clique com o botão direito no servidor e em **Copiar ID** para obter `GUILD_ID`. Faça o mesmo no canal do lembrete para obter `REMINDER_CHANNEL_ID`.

## Configuração

Variáveis de ambiente. Se existir um arquivo `.env` no diretório atual, ele é carregado (veja `.env.example`).

| Variável | Obrigatória | Padrão | Descrição |
|---|---|---|---|
| `DISCORD_TOKEN` | sim | | Token do bot |
| `GUILD_ID` | sim | | Servidor onde os comandos são registrados |
| `DB_PATH` | não | `./merges.db` (`/data/merges.db` no Docker) | Arquivo SQLite |
| `REMINDER_CHANNEL_ID` | não | | Canal do lembrete semanal. Vazio desliga o lembrete. |
| `REMINDER_CRON` | não | `0 9 * * 3` | Expressão cron (5 campos) do lembrete |
| `TZ_NAME` | não | `America/Sao_Paulo` | Fuso do cron e das datas exibidas |

Os comandos são registrados na guild (`ApplicationCommandBulkOverwrite`) toda vez que o bot sobe, então aparecem na hora.

## Rodando localmente

Requer Go 1.27+. Não precisa de CGO (o SQLite é `modernc.org/sqlite`).

```sh
cp .env.example .env   # preencha DISCORD_TOKEN e GUILD_ID
go run ./cmd/bot
```

Testes:

```sh
go test ./...
```

## Rodando com Docker

```sh
docker build -t merge-bot .
docker run -d --name merge-bot --restart unless-stopped \
  --env-file .env \
  -e DB_PATH=/data/merges.db \
  -v merge-bot-data:/data \
  merge-bot
```

O banco fica no volume `merge-bot-data`, então os pendentes sobrevivem a reinícios e atualizações da imagem. O bot encerra de forma limpa com `SIGINT`/`SIGTERM` (`docker stop`).

Se usar `--env-file`, deixe `DB_PATH` fora do `.env` ou aponte para `/data/...`; a linha `-e DB_PATH=...` acima garante isso.

## Estrutura

```
cmd/bot/main.go                 config, DB, sessão, registro de comandos, cron, shutdown
internal/config/config.go       leitura e validação das env vars
internal/store/store.go         acesso ao SQLite
internal/bot/commands.go        definição e registro dos slash commands
internal/bot/handlers.go        handlers de /merge, /pendentes e do botão
internal/bot/embeds.go          embeds, botões e formatação de "há X"
internal/reminder/reminder.go   lembrete semanal
```
