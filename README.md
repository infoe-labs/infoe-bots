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

## Container deployment

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
make deploy VERSION=v1.2.3
```

The deployment pulls the public GHCR image, resolves it to an immutable digest,
and runs it with a read-only filesystem, dropped capabilities, resource limits,
and no published network ports. If the replacement fails to start, the previous
container is restarted.

Create releases by pushing strict SemVer tags such as `v1.2.3`. GitHub-hosted
Actions test the code and publish the corresponding image to
`ghcr.io/infoe-labs/infoe-bots`.
