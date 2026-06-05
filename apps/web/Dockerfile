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

FROM public.ecr.aws/docker/library/alpine:3.21
RUN wget -qO /etc/apk/keys/nginx_signing.rsa.pub https://nginx.org/keys/nginx_signing.rsa.pub \
    && echo "https://nginx.org/packages/mainline/alpine/v3.21/main" >> /etc/apk/repositories \
    && apk add --no-cache nginx nginx-module-otel \
    && rm -rf /var/cache/apk/*

COPY apps/web/nginx-otel-main.conf /etc/nginx/nginx.conf
COPY apps/web/nginx.phase3.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/services/api/internal/httpapi/static /usr/share/nginx/html
RUN chown -R nginx:nginx /etc/nginx/conf.d /usr/share/nginx/html /var/cache/nginx /var/log/nginx /var/run
USER nginx
EXPOSE 8080
STOPSIGNAL SIGQUIT
CMD ["nginx", "-g", "daemon off;"]
