# syntax=docker/dockerfile:1.7

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/obsidian-polished ./cmd/obsidian-polished

FROM alpine:3.20
RUN apk add --no-cache ca-certificates git nginx openssl openssh

COPY --from=builder /out/obsidian-polished /usr/local/bin/obsidian-polished
COPY docker/start.sh /usr/local/bin/start.sh
COPY docker/nginx.conf.template /etc/nginx/http.d/default.conf.template

RUN chmod +x /usr/local/bin/start.sh \
    && mkdir -p /run/nginx /data/vault /data/out

ENV OBS_VAULT=/data/vault \
    OBS_OUT=/data/out \
    OBS_THEME=both \
    OBS_MAX_DEPTH=-1 \
    OBS_WATCH=true \
    OBS_WATCH_POLL=2s \
    OBS_WATCH_DEBOUNCE=1s \
    OBS_GIT_SYNC=false \
    OBS_GIT_PULL_INTERVAL=5m \
    OBS_GIT_REMOTE=origin \
    OBS_GIT_BRANCH= \
    OBS_GIT_SSH_KEY= \
    OBS_GIT_SSH_ACCEPT_NEW_HOST=false \
    OBS_SERVE_STATIC=false \
    OBS_HTTP_PORT=8080 \
    OBS_AUTH_ENABLED=false

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/start.sh"]
