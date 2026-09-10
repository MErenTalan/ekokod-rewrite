# syntax=docker/dockerfile:1

FROM node:24.20.0-alpine AS deps
WORKDIR /app
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
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
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 TZ=Europe/Istanbul
RUN adduser -D -u 10001 ekokod
COPY --from=build --chown=ekokod:ekokod /app/.next/standalone ./
COPY --from=build --chown=ekokod:ekokod /app/.next/static ./.next/static
COPY --from=build --chown=ekokod:ekokod /app/public ./public
USER ekokod
EXPOSE 3000
CMD ["node", "server.js"]
