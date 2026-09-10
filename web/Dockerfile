# syntax=docker/dockerfile:1

FROM node:24.20.0-alpine AS deps
WORKDIR /app
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

FROM node:24.20.0-alpine AS build
WORKDIR /app
RUN corepack enable
COPY --from=deps /app/node_modules ./node_modules
COPY web/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build

FROM node:24.20.0-alpine
WORKDIR /app
# HOSTNAME=0.0.0.0 overrides Docker's own auto-set HOSTNAME (the container
# ID). The standalone server.js binds to process.env.HOSTNAME, so without
# this it listens only on the container's own IP — never on 127.0.0.1 —
# and the healthcheck's `wget http://localhost:3000/...` below refuses the
# connection forever.
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 TZ=Europe/Istanbul HOSTNAME=0.0.0.0
RUN adduser -D -u 10001 ekokod
COPY --from=build --chown=ekokod:ekokod /app/.next/standalone ./
COPY --from=build --chown=ekokod:ekokod /app/.next/static ./.next/static
COPY --from=build --chown=ekokod:ekokod /app/public ./public
USER ekokod
EXPOSE 3000
CMD ["node", "server.js"]
