# Multi-stage build.
# The builder (golang:<version>, Debian trixie) and runtime-base (debian:trixie-slim)
# must share a glibc: pg_query_go requires CGO, so the binary is dynamically linked.
FROM golang:1.27.4 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -o /out/postgres-user-manager .

# Shared runtime base stage - Debian trixie slim matches the builder's glibc.
FROM debian:trixie-slim AS runtime-base

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd --system --no-create-home --home-dir /app --shell /usr/sbin/nologin appuser \
	&& install -d -m 0755 /app

WORKDIR /app

# Release stage used by GoReleaser (--target=release) with a pre-built binary.
FROM runtime-base AS release

COPY --chmod=0755 postgres-user-manager /usr/local/bin/postgres-user-manager

USER appuser

ENTRYPOINT ["/usr/local/bin/postgres-user-manager"]
CMD ["--help"]

# Default runtime stage for standalone source builds (docker build .).
FROM runtime-base AS runtime

COPY --from=builder --chmod=0755 /out/postgres-user-manager /usr/local/bin/postgres-user-manager

USER appuser

ENTRYPOINT ["/usr/local/bin/postgres-user-manager"]
CMD ["--help"]
