# Dockerfile for Kinetiq SvelteKit Web Dashboard (adapter-node production build)
FROM oven/bun:1-alpine AS build
WORKDIR /app
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile --ignore-scripts
COPY . .
# VITE_* are inlined at build time.
ARG VITE_API_URL
ARG VITE_AUTH_URL
ENV VITE_API_URL=$VITE_API_URL VITE_AUTH_URL=$VITE_AUTH_URL
RUN bun run build

FROM oven/bun:1-alpine
WORKDIR /app
COPY --from=build /app/build ./build
COPY --from=build /app/package.json ./
ENV NODE_ENV=production HOST=0.0.0.0 PORT=5173
EXPOSE 5173
CMD ["bun", "build/index.js"]
