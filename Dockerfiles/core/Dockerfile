# syntax=docker/dockerfile:1.6
#
# Multi-stage build for the BillionMail core service.
# Build context: repository root (see docker-compose.override.yml).

# ---------- Stage 1: frontend ----------
FROM node:22-alpine AS frontend
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY core/frontend/package.json core/frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY core/frontend ./
RUN pnpm build

# ---------- Stage 2: backend ----------
FROM golang:1.24-alpine AS backend
WORKDIR /src
RUN apk add --no-cache git build-base
COPY core/go.mod core/go.sum ./
RUN go mod download
COPY core ./
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
    -o /out/billionmail main.go

# ---------- Stage 3: runtime ----------
FROM alpine:3.20
LABEL maintainer="rahvis"
ENV LANG=C.UTF-8 LC_ALL=C.UTF-8

RUN apk add --no-cache \
        bash \
        ca-certificates \
        curl \
        supervisor \
        rsyslog \
        tzdata \
        busybox-extras \
        postgresql-client \
        fail2ban \
        iptables \
        ipset \
 && rm -rf /var/cache/apk/*

COPY --from=backend  /out/billionmail              /opt/billionmail/core/billionmail
COPY core/manifest                                 /opt/billionmail/core/manifest
COPY core/languages                                /opt/billionmail/core/languages
COPY core/public                                   /opt/billionmail/core/public
COPY --from=frontend /app/dist                     /opt/billionmail/core/public/dist
COPY core/resource                                 /opt/billionmail/core/resource
COPY core/template                                 /opt/billionmail/core/template

COPY Dockerfiles/core/stop-supervisor.sh   /stop-supervisor.sh
COPY Dockerfiles/core/core.sh              /core.sh
COPY Dockerfiles/core/restart_fail2ban.sh  /restart_fail2ban.sh
COPY Dockerfiles/core/supervisord.conf     /etc/supervisor/supervisord.conf
COPY Dockerfiles/core/fail2ban.conf        /etc/fail2ban/fail2ban.conf

RUN chmod +x /stop-supervisor.sh /core.sh /restart_fail2ban.sh \
             /opt/billionmail/core/billionmail

ENTRYPOINT ["/core.sh"]
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf"]
