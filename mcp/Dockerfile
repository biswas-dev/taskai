# Build stage — runs on host arch, produces platform-independent JS
FROM --platform=$BUILDPLATFORM node:24-alpine AS builder

WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci
COPY tsconfig.json ./
COPY src/ ./src/
RUN npm run build

# Production stage
FROM node:24-alpine

WORKDIR /app

# Non-root user
RUN addgroup -g 1001 -S taskai && \
    adduser -u 1001 -S taskai -G taskai

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --omit=dev

COPY --from=builder /app/dist ./dist

USER taskai

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --retries=3 --start-period=5s \
  CMD wget -q --spider http://localhost:3000/health || exit 1

CMD ["node", "dist/index.js"]
