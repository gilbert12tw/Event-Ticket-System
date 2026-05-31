# syntax=docker/dockerfile:1.7

FROM public.ecr.aws/docker/library/node:24.14.1-alpine3.22 AS builder

WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/web/package.json ./apps/web/
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --filter cets-web --store-dir /pnpm/store
COPY apps/web ./apps/web
RUN pnpm --filter cets-web build

FROM public.ecr.aws/docker/library/nginx:1.29-alpine

COPY apps/web/nginx.phase3.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/services/api/internal/httpapi/static /usr/share/nginx/html
EXPOSE 8080
