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

# Tugas maintenance berulang per klien. Jawabannya memuat klien_id, yang dipakai untuk catatan pekerjaan di bawah.
tugas() {
  printf '%s -> ' "$2"
  curl -sS -b "$JAR" -X POST "$BASE/api/tugas" -H 'Content-Type: application/json' -d "$1" \
    -w '\n%{http_code}\n' | tail -1
}
id_klien() {
  curl -sS -b "$JAR" "$BASE/api/klien" | tr '{' '\n' | grep "\"nama\":\"$1\"" | sed 's/.*"id":"\([^"]*\)".*/\1/' | head -1
}
id_tugas() {
  curl -sS -b "$JAR" "$BASE/api/tugas" | tr '{' '\n' | grep "\"judul\":\"$1\"" | sed 's/.*"id":"\([^"]*\)".*/\1/' | head -1
}

tugas "{\"klien\":\"Toko Kue Mawar\",\"judul\":\"Cek backup situs\",\"ulang_jumlah\":1,\"ulang_satuan\":\"bulan\",\"jatuh_tempo\":\"$(hari_lagi -2)\"}" "Tugas cek backup"
tugas "{\"klien\":\"Toko Kue Mawar\",\"judul\":\"Update plugin, tema, dan core\",\"ulang_jumlah\":1,\"ulang_satuan\":\"bulan\",\"jatuh_tempo\":\"$(hari_lagi +12)\"}" "Tugas update plugin"
tugas "{\"klien\":\"Kopi Senja\",\"judul\":\"Tinjau uptime dan sertifikat SSL\",\"ulang_jumlah\":3,\"ulang_satuan\":\"bulan\",\"jatuh_tempo\":\"$(hari_lagi +40)\"}" "Tugas tinjau uptime"
tugas "{\"klien\":\"Studio Arsitek Rana\",\"judul\":\"Tagih perpanjangan hosting dan domain\",\"ulang_jumlah\":1,\"ulang_satuan\":\"tahun\",\"jatuh_tempo\":\"$(hari_lagi +9)\",\"biaya\":1250000}" "Tugas tagih perpanjangan"

MAWAR=$(id_klien 'Toko Kue Mawar')
catat() {
  printf '%s -> ' "$2"
  curl -sS -b "$JAR" -o /dev/null -w '%{http_code}\n' -X POST "$BASE/api/aktivitas" -H 'Content-Type: application/json' -d "$1"
}
if [ -n "$MAWAR" ]; then
  catat "{\"klien_id\":\"$MAWAR\",\"jenis\":\"pekerjaan\",\"teks\":\"Mengganti foto banner promo Natal\",\"tanggal\":\"$(hari_lagi -3)\",\"durasi_menit\":35}" "Pekerjaan banner"
  catat "{\"klien_id\":\"$MAWAR\",\"jenis\":\"pekerjaan\",\"teks\":\"Memperbarui harga di halaman menu\",\"tanggal\":\"$(hari_lagi -1)\",\"durasi_menit\":20}" "Pekerjaan harga menu"
  catat "{\"klien_id\":\"$MAWAR\",\"jenis\":\"catatan\",\"teks\":\"Pemilik minta tagihan dikirim ke email bagian keuangan mulai Desember.\"}" "Catatan internal"
  BACKUP=$(id_tugas 'Cek backup situs')
  printf 'Tugas cek backup selesai -> '
  curl -sS -b "$JAR" -o /dev/null -w '%{http_code}\n' -X POST "$BASE/api/tugas/$BACKUP/selesai" -H 'Content-Type: application/json' \
    -d '{"catatan":"Backup terakhir lengkap dan berhasil dipulihkan di staging.","durasi_menit":25}'
  printf 'Ringkasan laporan bulan ini -> '
  curl -sS -b "$JAR" -o /dev/null -w '%{http_code}\n' -X PUT "$BASE/api/klien/$MAWAR/laporan/catatan" -H 'Content-Type: application/json' \
    -d "{\"bulan\":\"$(date +%Y-%m)\",\"teks\":\"Situs berjalan normal bulan ini. Domain tokokuemawar.id perlu diperpanjang minggu ini; tagihannya sudah kami kirim.\"}"
fi
