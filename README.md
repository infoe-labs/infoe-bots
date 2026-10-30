# InfOE Bots

Run every bot feature in one Discord session:

```sh
go run main.go --discord-token "$DISCORD_TOKEN" all
```

Individual features remain available as subcommands:

```sh
go run main.go --discord-token "$DISCORD_TOKEN" ping
go run main.go --discord-token "$DISCORD_TOKEN" mango
go run main.go --discord-token "$DISCORD_TOKEN" urlsanitizer
go run main.go --discord-token "$DISCORD_TOKEN" llmpromoter
```

## Binary deployment

Store the Discord token in a local file, not in this repository or directly in
`.env`:

```sh
mkdir -p .secrets
printf '%s' "$DISCORD_TOKEN" > .secrets/discord-token
chmod 600 .secrets/discord-token
cp .env.example .env
```

Set `DISCORD_TOKEN_FILE` in `.env` to the absolute path of that token file.
Then deploy a published SemVer release:

```sh
make deploy
```

With no `VERSION`, deployment discovers the newest strict SemVer release tag on
`origin`, downloads the Linux/amd64 binary and checksum, verifies both the
checksum and embedded version, and asks for confirmation before replacing the
running process. To deploy a known version without the interactive confirmation,
specify it explicitly:

```sh
make deploy VERSION=v1.2.3
```

The deployment installs the binary under `~/.local/bin` and runs it in a detached
`screen` session named `infoe-bots`. Output is written under
`~/.local/state/infoe-bots`. If the replacement exits during startup, the previous
binary is restored and restarted.

Check the deployed version and process state, or follow the bot's stdout/stderr:

```sh
make status
make logs
```

Press `Ctrl-C` to stop following logs; this does not stop the bot.

Create releases by pushing strict SemVer tags such as `v1.2.3`. GitHub-hosted
Actions test the code and publish the corresponding binary and SHA-256 checksum
as GitHub Release assets.
