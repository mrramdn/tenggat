# ---------- Stage 1: build ----------
# Node lengkap hanya dipakai untuk memasang dependency dan membangun aset statis.
FROM node:24.22.2-alpine3.23 AS build

WORKDIR /app

# pnpm dengan versi yang sama dengan lockfile (lihat "packageManager" di package.json).
RUN npm install --global pnpm@12.8.1

# File dependency disalin lebih dulu: selama ketiga file ini tidak berubah,
# layer "pnpm install" diambil dari cache.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile

# Baru kemudian source code, lalu build menjadi file statis di folder dist/.
COPY . .
RUN pnpm build

# ---------- Stage 2: runtime ----------
# nginx versi unprivileged berjalan sebagai user biasa (bukan root) di port 8080.
# Node, node_modules, dan source code tidak ikut; hanya hasil build dan konfigurasi.
FROM nginxinc/nginx-unprivileged:1.30.6-alpine3.24 AS runtime

WORKDIR /usr/share/nginx/html

# Membungkam log pembuka dari skrip entrypoint image nginx.
ENV NGINX_ENTRYPOINT_QUIET_LOGS=1

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist ./

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD ["wget", "-q", "--spider", "http://127.0.0.1:8080/"]

CMD ["nginx", "-g", "daemon off;"]
