FROM node:22-alpine AS builder
WORKDIR /workspace
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY src/apps/shared/package.json src/apps/shared/package.json
COPY src/apps/console-lite/package.json src/apps/console-lite/package.json
RUN pnpm install --frozen-lockfile
COPY src/apps/shared src/apps/shared
COPY src/apps/console-lite src/apps/console-lite
RUN pnpm --filter console-lite build

FROM nginx:1.27-alpine
COPY src/apps/shared/nginx-spa.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /workspace/src/apps/console-lite/dist /usr/share/nginx/html
HEALTHCHECK CMD wget -qO- http://127.0.0.1/healthz >/dev/null || exit 1
