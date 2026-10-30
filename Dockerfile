# syntax=docker/dockerfile:1
FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build

ARG VERSION=dev
ARG VCS_REF=unknown

RUN apk add --no-cache build-base
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build \
    -trimpath \
    -ldflags="-s -w -buildid= -X github.com/infoe-labs/infoe-bots/cli.Version=${VERSION}" \
    -o /out/infoe-bots ./main.go

FROM alpine:3.21@sha256:ee16d92f96db08d9d2d07da48555a27f3a69d8f0759021dbc601195ed0d2b5bd

ARG VERSION=dev
ARG VCS_REF=unknown
LABEL org.opencontainers.image.source="https://github.com/infoe-labs/infoe-bots" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}"

RUN apk add --no-cache ca-certificates \
    && addgroup -S infoe \
    && adduser -S -G infoe -h /home/infoe infoe \
    && mkdir -p /home/infoe/.infoe-bots \
    && chown -R infoe:infoe /home/infoe

COPY --from=build --chown=infoe:infoe /out/infoe-bots /usr/local/bin/infoe-bots

USER infoe
WORKDIR /home/infoe
ENTRYPOINT ["/usr/local/bin/infoe-bots"]
CMD ["--discord-token-file", "/run/secrets/discord_token", "all"]
