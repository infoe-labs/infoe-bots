SHELL := /bin/sh
.DEFAULT_GOAL := help

-include .env

IMAGE ?= ghcr.io/infoe-labs/infoe-bots
CONTAINER ?= infoe-bots
DATA_VOLUME ?= infoe-bots-data

.PHONY: help test build deploy

help:
	@printf '%s\n' \
	  'make test' \
	  'make build VERSION=v1.2.3' \
	  'make deploy  # confirm the latest release' \
	  'make deploy VERSION=v1.2.3'

test:
	go test ./...
	go vet ./...

build:
	@printf '%s' '$(VERSION)' | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$$' || { echo 'VERSION must be strict SemVer, for example v1.2.3' >&2; exit 2; }
	docker build --build-arg VERSION='$(VERSION)' --build-arg VCS_REF="$$(git rev-parse HEAD)" -t '$(IMAGE):$(VERSION)' .

deploy:
	@set -eu; \
	version='$(VERSION)'; \
	confirm=false; \
	if [ -z "$$version" ]; then \
	  version=$$(git ls-remote --tags --refs origin 'refs/tags/v*' \
	    | sed 's#.*refs/tags/##' \
	    | grep -E '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$$' \
	    | sort -V \
	    | tail -n 1); \
	  [ -n "$$version" ] || { echo 'No strict SemVer release tags found on origin' >&2; exit 2; }; \
	  confirm=true; \
	fi; \
	printf '%s' "$$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$$' || { echo 'VERSION must be strict SemVer, for example v1.2.3' >&2; exit 2; }; \
	token_file='$(DISCORD_TOKEN_FILE)'; \
	[ -n "$$token_file" ] || { echo 'Set DISCORD_TOKEN_FILE in .env or on the make command line' >&2; exit 2; }; \
	[ -f "$$token_file" ] || { echo 'DISCORD_TOKEN_FILE does not exist' >&2; exit 2; }; \
	mode=$$(stat -c '%a' "$$token_file"); \
	[ "$$mode" = 600 ] || { echo 'Discord token file must have mode 0600' >&2; exit 2; }; \
	docker pull '$(IMAGE):'"$$version"; \
	image=$$(docker image inspect --format '{{index .RepoDigests 0}}' '$(IMAGE):'"$$version"); \
	if [ "$$confirm" = true ]; then \
	  printf 'Deploy latest release %s (%s)? [y/N] ' "$$version" "$$image"; \
	  read -r answer; \
	  case "$$answer" in y|Y|yes|YES) ;; *) echo 'Deployment cancelled'; exit 0 ;; esac; \
	fi; \
	docker rm -f '$(CONTAINER)-next' >/dev/null 2>&1 || true; \
	had_old=false; \
	if docker inspect '$(CONTAINER)' >/dev/null 2>&1; then had_old=true; docker stop '$(CONTAINER)' >/dev/null; fi; \
	if docker run -d \
	  --name '$(CONTAINER)-next' \
	  --restart unless-stopped \
	  --read-only \
	  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
	  --cap-drop ALL \
	  --security-opt no-new-privileges:true \
	  --pids-limit 100 \
	  --memory 128m \
	  --cpus 0.50 \
	  --mount type=volume,src='$(DATA_VOLUME)',dst=/home/infoe/.infoe-bots \
	  --mount type=bind,src="$$token_file",dst=/run/secrets/discord_token,readonly \
	  "$$image" >/dev/null; then \
	  sleep 3; \
	  if [ "$$(docker inspect --format '{{.State.Running}}' '$(CONTAINER)-next')" = true ]; then \
	    docker rm '$(CONTAINER)' >/dev/null 2>&1 || true; \
	    docker rename '$(CONTAINER)-next' '$(CONTAINER)'; \
	    echo "Deployed $$image"; \
	  else \
	    docker logs --tail 20 '$(CONTAINER)-next' >&2 || true; \
	    docker rm -f '$(CONTAINER)-next' >/dev/null 2>&1 || true; \
	    if [ "$$had_old" = true ]; then docker start '$(CONTAINER)' >/dev/null; fi; \
	    exit 1; \
	  fi; \
	else \
	  if [ "$$had_old" = true ]; then docker start '$(CONTAINER)' >/dev/null; fi; \
	  exit 1; \
	fi
