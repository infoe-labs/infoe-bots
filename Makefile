SHELL := /bin/sh
.DEFAULT_GOAL := help

-include .env

REPOSITORY ?= infoe-labs/infoe-bots
BINARY ?= $(HOME)/.local/bin/infoe-bots
SESSION ?= infoe-bots
LOG_FILE ?= $(HOME)/.local/state/infoe-bots/infoe-bots.log

.PHONY: help test build deploy status logs

help:
	@printf '%s\n' \
	  'make test' \
	  'make build VERSION=v1.2.3' \
	  'make deploy  # confirm the latest release' \
	  'make deploy VERSION=v1.2.3' \
	  'make status' \
	  'make logs'

test:
	go test ./...
	go vet ./...

build:
	@printf '%s' '$(VERSION)' | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$$' || { echo 'VERSION must be strict SemVer, for example v1.2.3' >&2; exit 2; }
	go build -trimpath -ldflags='-s -w -buildid= -X github.com/infoe-labs/infoe-bots/cli.Version=$(VERSION)' -o infoe-bots .

deploy:
	@set -eu; \
	umask 077; \
	command -v curl >/dev/null || { echo 'curl is required' >&2; exit 2; }; \
	command -v screen >/dev/null || { echo 'screen is required' >&2; exit 2; }; \
	version='$(VERSION)'; \
	confirm=false; \
	if [ -z "$$version" ]; then \
	  version=$$(git ls-remote --tags --refs origin 'refs/tags/v*' \
	    | sed 's#.*refs/tags/##' \
	    | grep -E '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$$' \
	    | sort -V \
	    | tail -n 1); \
	  [ -n "$$version" ] || { echo 'No strict SemVer release tags found on origin' >&2; exit 2; }; \
	  confirm=true; \
	fi; \
	printf '%s' "$$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$$' || { echo 'VERSION must be strict SemVer, for example v1.2.3' >&2; exit 2; }; \
	token_file='$(DISCORD_TOKEN_FILE)'; \
	[ -n "$$token_file" ] || { echo 'Set DISCORD_TOKEN_FILE in .env or on the make command line' >&2; exit 2; }; \
	[ -f "$$token_file" ] || { echo 'DISCORD_TOKEN_FILE does not exist' >&2; exit 2; }; \
	mode=$$(stat -c '%a' "$$token_file"); \
	[ "$$mode" = 600 ] || { echo 'Discord token file must have mode 0600' >&2; exit 2; }; \
	tmp_dir=$$(mktemp -d); \
	trap 'rm -rf "$$tmp_dir"' EXIT; \
	trap 'exit 130' INT; \
	trap 'exit 143' HUP TERM; \
	asset_url="https://github.com/$(REPOSITORY)/releases/download/$$version/infoe-bots-linux-amd64"; \
	echo "Downloading $$version release assets..."; \
	curl --fail --location --silent --show-error --proto '=https' --tlsv1.2 --output "$$tmp_dir/infoe-bots-linux-amd64" "$$asset_url"; \
	curl --fail --location --silent --show-error --proto '=https' --tlsv1.2 --output "$$tmp_dir/infoe-bots-linux-amd64.sha256" "$$asset_url.sha256"; \
	echo 'Verifying checksum and embedded version...'; \
	(cd "$$tmp_dir" && sha256sum --check infoe-bots-linux-amd64.sha256); \
	chmod 755 "$$tmp_dir/infoe-bots-linux-amd64"; \
	actual_version=$$("$$tmp_dir/infoe-bots-linux-amd64" --version); \
	[ "$$actual_version" = "infoe version $$version" ] || { echo "Downloaded binary reports unexpected version: $$actual_version" >&2; exit 1; }; \
	if [ "$$confirm" = true ]; then \
	  checksum=$$(cut -d ' ' -f 1 "$$tmp_dir/infoe-bots-linux-amd64.sha256"); \
	  printf 'Deploy latest release %s (sha256:%s)? [y/N] ' "$$version" "$$checksum"; \
	  read -r answer; \
	  case "$$answer" in y|Y|yes|YES) ;; *) echo 'Deployment cancelled'; exit 0 ;; esac; \
	fi; \
	binary='$(BINARY)'; \
	log_file='$(LOG_FILE)'; \
	mkdir -p "$$(dirname "$$binary")" "$$(dirname "$$log_file")"; \
	: > "$$log_file"; \
	chmod 600 "$$log_file"; \
	echo "Installing $$binary..."; \
	had_old=false; \
	if [ -f "$$binary" ]; then cp -p "$$binary" "$$binary.previous"; had_old=true; fi; \
	mv "$$tmp_dir/infoe-bots-linux-amd64" "$$binary.new"; \
	mv "$$binary.new" "$$binary"; \
	if screen -ls 2>/dev/null | grep -Eq '[.]$(SESSION)[[:space:]]'; then \
	  echo 'Stopping the previous screen session...'; \
	  screen -S '$(SESSION)' -X quit; \
	fi; \
	echo 'Starting the bot in a detached screen session...'; \
	if screen -L -Logfile "$$log_file" -dmS '$(SESSION)' "$$binary" --discord-token-file "$$token_file" all; then \
	  echo 'Waiting 3 seconds for the startup check...'; \
	  sleep 3; \
	  if screen -ls 2>/dev/null | grep -Eq '[.]$(SESSION)[[:space:]]'; then \
	    rm -f "$$binary.previous"; \
	    echo "Deployed $$version in screen session $(SESSION)"; \
	    echo "Log: $$log_file"; \
	    exit 0; \
	  fi; \
	fi; \
	echo 'New process exited during startup' >&2; \
	if [ "$$had_old" = true ]; then \
	  mv "$$binary.previous" "$$binary"; \
	  screen -L -Logfile "$$log_file" -dmS '$(SESSION)' "$$binary" --discord-token-file "$$token_file" all; \
	  echo 'Restored the previous binary' >&2; \
	fi; \
	exit 1

status:
	@if [ -x '$(BINARY)' ]; then '$(BINARY)' --version; else echo 'Binary is not installed'; fi
	@if screen -ls 2>/dev/null | grep -Eq '[.]$(SESSION)[[:space:]]'; then echo 'Screen session $(SESSION) is running'; else echo 'Screen session $(SESSION) is not running'; exit 1; fi

logs:
	@[ -f '$(LOG_FILE)' ] || { echo 'No log file exists yet' >&2; exit 1; }
	tail -n 100 -f '$(LOG_FILE)'
