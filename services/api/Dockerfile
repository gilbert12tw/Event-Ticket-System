# syntax=docker/dockerfile:1.7

FROM node:24.14.1-alpine3.22 AS web-builder

WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/web/package.json ./apps/web/
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --filter cets-web --store-dir /pnpm/store
COPY apps/web ./apps/web
RUN pnpm --filter cets-web build

FROM golang:1.25.9-alpine3.22 AS builder

WORKDIR /src/services/api
COPY services/api/go.mod services/api/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY services/api ./
COPY --from=web-builder /src/services/api/internal/httpapi/static ./internal/httpapi/static
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /out/cets ./cmd/cets

FROM alpine:3.22.4

RUN addgroup -S cets && adduser -S -G cets cets
WORKDIR /app
COPY --from=builder /out/cets /app/cets
USER cets
EXPOSE 8080
ENTRYPOINT ["/app/cets"]
