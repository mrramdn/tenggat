# Tenggat

Masa berlaku domain, sertifikat TLS, token API, dan langganan dalam satu daftar. Tenggat membaca tanggal sertifikat
langsung dari server dan mengingatkan sebelum ada yang kedaluwarsa.

Status: dalam pengembangan.

## Struktur

- `api/`: layanan Go (REST API, PostgreSQL, Redis)
- `web/`: antarmuka React yang disajikan nginx
