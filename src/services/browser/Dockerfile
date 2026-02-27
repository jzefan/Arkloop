# 构建时使用 node:22-alpine 安装依赖并编译 TypeScript
FROM node:22-alpine AS builder
WORKDIR /app

RUN npm install -g pnpm

COPY src/services/browser/package.json src/services/browser/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY src/services/browser/src ./src
COPY src/services/browser/tsconfig.json ./
RUN pnpm run build

# 运行时基于官方 Playwright 镜像（预装 Chromium 及全部系统依赖）
FROM mcr.microsoft.com/playwright:v1.50.0-noble AS runtime
WORKDIR /app

RUN npm install -g pnpm

COPY src/services/browser/package.json src/services/browser/pnpm-lock.yaml ./
RUN pnpm install --prod --frozen-lockfile

COPY --from=builder /app/dist ./dist

ENV NODE_ENV=production
ENV BROWSER_PORT=3000

EXPOSE 3000

CMD ["node", "dist/index.js"]
