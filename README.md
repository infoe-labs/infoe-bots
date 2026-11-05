# InfOE Bots

Run every bot feature in one Discord session:

```sh
go run main.go --discord-token-file .secrets/discord-token all
```

Individual features remain available as subcommands:

```sh
go run main.go --discord-token-file .secrets/discord-token ping
go run main.go --discord-token-file .secrets/discord-token urlsanitizer
go run main.go --discord-token-file .secrets/discord-token urlstripper
go run main.go --discord-token-file .secrets/discord-token llmpromoter
```

`mango` is a preserved legacy joke subcommand. It is not included in `all` and
runs only when explicitly invoked:

```sh
go run main.go --discord-token-file .secrets/discord-token mango
```

## URL sanitizer

`urlsanitizer` rewrites social-media links for safer sharing and better embeds:
Instagram reels become `kkinstagram` links, X/Twitter links become `fxtwitter`
links, Facebook `/share/` links are removed with guidance to post a direct link,
and profile-exposing TikTok links get a warning. Links carrying analytics
parameters also get a tracking-free copy (see below); that part runs on its own as
the `urlstripper` mode.

### Tracking-free links (no-analytics mode)

When a message contains a link whose query string exists only to attribute the
visit, the bot adds a new message to the same channel with the same link, cleaned:

```text
🧹 <@someone>, here are the same links without tracking parameters:

https://cybernews.com/security/europe-supermarket-refrigerator-hacks/

-# Sent · the original poster can react with :x: to delete this message.
```

Unlike the Instagram and X/Twitter rewrites, this mode never deletes anything. The
original message stays exactly where it was; the bot only adds a reply carrying
the clean links, so nobody has to trust the bot with someone else's post.

Removed parameters are the ones that carry attribution rather than page content:
the `utm_`, `mtm_`, `hsa_`, `mc_`, `sfmc_`, `sg_`, `at_`, `trk_`, `klaviyo_`,
`vero_` and `dcm_` prefixes, plus named click ids and referrer tags such as
`fbclid`, `gclid`, `gbraid`, `wbraid`, `msclkid`, `ttclid`, `igsh`, `epst`,
`_gl`, `si` and `ref`. The un-prefixed twins `source`, `medium`, `campaign`,
`term` and `content` are removed only when the same link already carries a
`utm_*` parameter, so sites that use those names for real content keep working.

Guarantees:

- the triggering message is never deleted, edited, or hidden
- parameters that are not on the tracking list, and any URL fragment, are kept
  verbatim, so `?id=7`, `?q=go+language` and `#section` survive intact
- a link is reposted only if something was actually removed; a link with no
  tracking parameters produces no message at all
- a link that cannot be parsed unambiguously is left alone rather than reposted
  in a mangled form
- several links in one message produce one reply listing each clean link once,
  and only human messages are considered, so bot and webhook posts are not
  echoed back at them
- like the other reposts, the message carries the author's ID in its bot-generated
  header, so only the original poster can `:x:` it away
- nothing the bot posts pings anybody: mentions are switched off on these
  messages, including the ping a reply would otherwise send

Stripping is done locally by parsing the URL; the bot never fetches the link's
destination. When the X/Twitter rewrite does replace a message, any other link in
the same message is cleaned the same way before it is carried into the repost.

### Deleting a repost by reaction

When the bot replaces an Instagram reel or X/Twitter link with a rewritten
version, or adds a tracking-free copy of a link, the replacement message offers
self-service deletion. The bot posts the message and pre-seeds a ❌ reaction so it
can be clicked:

```text
🔗 Converted to fxtwitter for better embeds (original message from @someone)

https://fxtwitter.com/someone/status/123

-# Sent · the original poster can react with :x: to delete this message.
```

Only the original author can delete their own repost. The author's Discord ID is
carried inside the replacement message itself, in the bot-generated header, so
authorization is a text comparison against the message rather than a database
lookup. No state is stored locally and the rule keeps working after a bot
restart. The footer deliberately names no one, so the poster is not mentioned
twice, and every repost is sent with mentions unparsed, which also drops the ping
that the tracking-free reply would otherwise send.

A reaction deletes a message only when all of the following hold:

- the reaction is the Unicode ❌ emoji, not a similarly named custom server emoji
- the reaction was added by someone other than the bot
- the reacted message was authored by this bot
- the message begins with one of the bot-generated repost headers naming that
  same user
- the message ends with the deletion footer, or with the older footer that named
  that same user, so reposts written before the wording changed stay deletable

The header and footer are matched as an anchored prefix and suffix, so a user
cannot delete someone else's repost by copying the instruction into their own
message, and a `:x:` reaction on any unrelated message is ignored. Deleting
removes only the bot's replacement message, never the message that prompted it.

## Releasing a version

A release is a strict SemVer tag on the default branch. Pushing one is the whole
publish step: the `Publish release` workflow builds the Linux/amd64 binary, its
SHA-256 checksum, and a build-provenance attestation, then opens the matching
GitHub Release, which is what `make deploy` later installs.

After a pull request merges, cut the next release with:

```sh
make release            # patch bump: v0.2.0 -> v0.2.1
make release BUMP=minor # minor bump: v0.2.0 -> v0.3.0
make release BUMP=major # major bump: v0.2.0 -> v1.0.0
```

The version is derived from the newest strict SemVer tag on `origin`, using the
same ordering `make deploy` uses to pick the newest release, so the two cannot
disagree about which version is current. Prerelease tags such as `v1.0.0-rc.1`
contribute their stable core; tags the workflow could never publish (`v1.2`,
`v1.2.3.4`) are ignored rather than bumped. Choose an exact version instead of a
bump with `make release VERSION=v1.2.3`, and set `MESSAGE="..."` to give the
annotated tag a description instead of the bare version.

Before anything is written, the release is checked and shown:

```text
base      v0.2.0
release   v0.3.0 (minor bump)
tagger    <the repository-local git identity>
2 commit(s) since v0.2.0:
  653f851 Add a no-analytics mode that reposts links instead of deleting them (#11)
  10ff9df Frame the repost footer as a deletion option for the original poster (#10)
Tag v0.3.0 and publish it? [y/N]
```

Nothing is published until you answer `y`, and a published release cannot be
quietly undone, so the checks are deliberately strict. The release is refused when
the working tree has uncommitted changes, when HEAD is detached or on any branch
other than the default branch, when HEAD is ahead of or behind `origin/main`, when
the tag already exists locally or on `origin`, when the version does not trail the
newest release, when the tagger identity is not the repository-local one, or when
the newest tag already points at HEAD (nothing was merged since the last release).
`make release` runs `make test` first, because a failing build should never reach
the point of publishing.

Preview a release without touching the repository:

```sh
DRY_RUN=true sh scripts/release.sh minor
```

To release from a non-interactive shell, where the prompt cannot be answered, pass
`ASSUME_YES=yes` once you have confirmed the version yourself. Override `ORIGIN` or
`DEFAULT_BRANCH` to release from a fork or a release branch. `make build-release`
is an alias for `make release`; it publishes by pushing the tag, so no separate
build step exists.

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

Because deployment installs a published tag rather than the tip of the default
branch, work can be merged and released-pending without ever reaching the running
bot. To make that visible, `make deploy` prints a drift warning before installing:

```text
warning: release v0.1.1 is 3 commits behind origin/main
```

The check is advisory: it never blocks or alters a deployment, and it stays silent
when the comparison cannot be made (for example offline, or when the default branch
cannot be determined). It compares against the default branch known to your local
repository, refreshing the tag and branch refs from `origin` on a best-effort
basis first, since `make deploy` otherwise discovers tags with `git ls-remote`
without downloading them. Override the comparison with `ORIGIN` and
`DEFAULT_BRANCH`, or run it directly:

```sh
sh scripts/deploy-drift.sh v0.1.1
```

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

Repost deletion works without storage by embedding the author's Discord ID in the
replacement message text sent to Discord as a mention. That identifier lives in
the Discord message itself and is never written to local feature data. Bot
messages are sent with mention parsing disabled, so the bot never pings the
author or the person it replies to.

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
connects to GitHub to discover and download release assets. Social-media URLs are
rewritten and tracking parameters are stripped locally; the bot does not fetch
their destinations.

Tokens are accepted only through a mode-`0600` file. Passing a token directly on
the command line is intentionally unsupported because command arguments may be
retained in shell history or exposed through process listings.

Releases are published by the tag push described in Releasing a version: the
GitHub-hosted workflow tests the code and publishes the binary and SHA-256
checksum as GitHub Release assets.

Cooldown history stays in memory for six hours and is removed by a cleanup task
that runs every minute, including while the bot is idle. Expired entries are also
removed when recording a new response. Opt-out preferences do not expire, so
cleanup never opts users back into unwanted messages.
