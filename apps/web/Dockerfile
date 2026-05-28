FROM node:24-alpine AS build

WORKDIR /src

RUN corepack enable

COPY pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/web/package.json ./apps/web/package.json

RUN pnpm install --filter web --frozen-lockfile

COPY apps/web ./apps/web

RUN pnpm -F web build

FROM caddy:2-alpine

COPY --from=build /src/apps/web/dist /srv/web
COPY apps/web/Caddyfile /etc/caddy/Caddyfile

EXPOSE 80

CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
