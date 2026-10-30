# InfOE Bots

Run every bot feature in one Discord session:

```sh
go run main.go --discord-token-file .secrets/discord-token all
```

Individual features remain available as subcommands:

```sh
go run main.go --discord-token-file .secrets/discord-token ping
go run main.go --discord-token-file .secrets/discord-token mango
go run main.go --discord-token-file .secrets/discord-token urlsanitizer
go run main.go --discord-token-file .secrets/discord-token llmpromoter
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

## Privacy and retained data

The bot processes Discord events in memory to provide its configured features.
It does not persist message content, usernames, guild IDs, channel IDs, message
IDs, IP addresses, or raw Discord user IDs.

DiscordGo's guild, channel, member, and private-channel state caches are disabled;
only the bot's own connection identity is retained by the library. DiscordGo's
internal diagnostic logger is also disabled because upstream diagnostics may
include event payloads or Discord resource identifiers.

The only persistent feature data is the LLM-promoter opt-out set. Each entry
contains a feature name and an HMAC pseudonym derived from the Discord user ID
using the locally supplied bot token as the key. The token and raw user ID are
not written to this data file. Token rotation intentionally makes previous
pseudonyms unlinkable. Cooldown timestamps are held in memory only and disappear
when the bot restarts.

Local feature data is stored under `~/.infoe-bots` with directory mode `0700`
and file mode `0600`. On startup, legacy data is migrated in place: raw user IDs
are pseudonymized and historical contact timestamps are discarded.

Operational output contains lifecycle messages and generic Discord operation
categories only. It does not log message content, usernames, user IDs, Discord
API response bodies, or tokens. `make deploy` resets the mode-`0600` operational
log before starting a replacement process.

At runtime, the bot connects only to Discord's gateway and API. Deployment
connects to GitHub to discover and download release assets. Social-media URLs
are rewritten locally; the bot does not fetch their destinations.

Tokens are accepted only through a mode-`0600` file. Passing a token directly on
the command line is intentionally unsupported because command arguments may be
retained in shell history or exposed through process listings.

Create releases by pushing strict SemVer tags such as `v1.2.3`. GitHub-hosted
Actions test the code and publish the corresponding binary and SHA-256 checksum
as GitHub Release assets.
