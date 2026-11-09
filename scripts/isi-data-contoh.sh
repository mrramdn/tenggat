#!/usr/bin/env sh
# Mengisi Tenggat dengan data contoh lewat API, supaya tampilan bisa dicoba.
# Membuat akun contoh (atau masuk bila sudah ada), lalu menambah aset atas nama akun itu.
#
# Pemakaian: ./scripts/isi-data-contoh.sh [alamat]   (bawaan http://localhost:8080)
# Akun bisa diganti lewat env: TENGGAT_EMAIL, TENGGAT_SANDI, TENGGAT_NAMA.
# Nama klien dan aset manual di sini fiktif; sertifikat dibaca dari situs publik.
set -eu

BASE="${1:-http://localhost:8080}"
EMAIL="${TENGGAT_EMAIL:-contoh@tenggat.id}"
SANDI="${TENGGAT_SANDI:-contoh-tenggat}"
NAMA="${TENGGAT_NAMA:-Akun Contoh}"
JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT

# Tanggal relatif terhadap hari ini, untuk date GNU (Linux) maupun BSD (macOS).
hari_lagi() {
  date -d "$1 days" +%F 2>/dev/null || date -v"$1"d +%F
}

kode=$(curl -sS -c "$JAR" -o /dev/null -w '%{http_code}' -X POST "$BASE/api/akun" \
  -H 'Content-Type: application/json' \
  -d "{\"nama\":\"$NAMA\",\"email\":\"$EMAIL\",\"kata_sandi\":\"$SANDI\"}")
if [ "$kode" = 409 ]; then
  kode=$(curl -sS -c "$JAR" -o /dev/null -w '%{http_code}' -X POST "$BASE/api/sesi" \
    -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"kata_sandi\":\"$SANDI\"}")
fi
case "$kode" in
  200|201) echo "masuk sebagai $EMAIL" ;;
  *) echo "gagal masuk sebagai $EMAIL (HTTP $kode)" >&2; exit 1 ;;
esac

kirim() {
  printf '%s -> ' "$2"
  curl -sS -b "$JAR" -o /dev/null -w '%{http_code}\n' -X POST "$BASE/api/item" \
    -H 'Content-Type: application/json' -d "$1"
}

kirim "{\"nama\":\"Domain tokokuemawar.id\",\"jenis\":\"domain\",\"klien\":\"Toko Kue Mawar\",\"host\":\"tokokuemawar.id\",\"kedaluwarsa\":\"$(hari_lagi +4)\",\"biaya\":285000}" "Domain tokokuemawar.id"
kirim "{\"nama\":\"Token GitHub Actions deploy\",\"jenis\":\"token\",\"klien\":\"Pribadi\",\"kedaluwarsa\":\"$(hari_lagi +6)\"}" "Token GitHub Actions"
kirim "{\"nama\":\"Langganan VPS 2 vCPU\",\"jenis\":\"langganan\",\"klien\":\"Pribadi\",\"kedaluwarsa\":\"$(hari_lagi +22)\",\"biaya\":189000}" "Langganan VPS"
kirim "{\"nama\":\"Domain kopisenja.com\",\"jenis\":\"domain\",\"klien\":\"Kopi Senja\",\"host\":\"kopisenja.com\",\"kedaluwarsa\":\"$(hari_lagi +72)\",\"biaya\":210000}" "Domain kopisenja.com"
kirim "{\"nama\":\"Token Cloudflare DNS\",\"jenis\":\"token\",\"klien\":\"Pribadi\",\"kedaluwarsa\":\"$(hari_lagi +181)\"}" "Token Cloudflare DNS"
kirim "{\"nama\":\"Lisensi font editorial\",\"jenis\":\"lainnya\",\"klien\":\"Studio Arsitek Rana\",\"kedaluwarsa\":\"$(hari_lagi -3)\",\"biaya\":450000}" "Lisensi font (sudah lewat)"
kirim '{"jenis":"sertifikat","klien":"Contoh publik","host":"example.com"}' "Sertifikat example.com"
kirim '{"jenis":"sertifikat","klien":"Contoh publik","host":"wikipedia.org"}' "Sertifikat wikipedia.org"
