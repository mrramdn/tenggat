# ---------- Stage 1: build ----------
# Image Go lengkap (compiler dan alat build), hanya dipakai untuk mengompilasi.
FROM golang:1.27.3-alpine3.24 AS build

WORKDIR /src

# Binary statis tanpa cgo, sehingga bisa jalan di image yang tidak punya libc.
ENV CGO_ENABLED=0 \
    GOOS=linux

# File dependency disalin lebih dulu: selama go.mod dan go.sum tidak berubah,
# layer "go mod download" diambil dari cache dan tidak diunduh ulang.
COPY go.mod go.sum ./
RUN go mod download

# Baru kemudian seluruh source code, lalu compile.
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- Stage 2: runtime ----------
# distroless/static hanya berisi sertifikat CA, data zona waktu, dan user nonroot:
# tanpa shell, package manager, atau compiler. Distroless tidak punya tag versi,
# jadi image dipin dengan digest agar setiap build memakai isi yang sama persis.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329 AS runtime

WORKDIR /app

ENV PORT=8080 \
    CEK_INTERVAL=6h

# Hanya binary hasil stage build yang dibawa ke image akhir.
COPY --from=build /out/server /app/server

EXPOSE 8080

USER nonroot:nonroot

# Image ini tidak punya curl atau wget, jadi binary memeriksa dirinya sendiri.
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/app/server", "healthcheck"]

ENTRYPOINT ["/app/server"]
