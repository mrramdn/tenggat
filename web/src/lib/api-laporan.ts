// Monthly client reports (owner side and the public read-only link) and the owner's notification webhook.
import { request } from './api'
import type { Jenis, Status, Sumber } from './api'

export interface BarisPekerjaan {
  tanggal: string // YYYY-MM-DD
  teks: string
  durasi_menit: number | null
  /** 'pekerjaan' (a work entry) or 'tugas_selesai' (a recurring task marked done). */
  jenis: string
}

export interface BarisPerpanjangan {
  tanggal: string
  teks: string
}

export interface AsetLaporan {
  nama: string
  jenis: Jenis
  host: string | null
  kedaluwarsa: string
  sisa_hari: number
  status: Status
  sumber: Sumber
  biaya: number | null
}

export interface JatuhTempo {
  nama: string
  /** An asset type, or 'tugas' for a recurring task. */
  jenis: Jenis | 'tugas' | string
  tanggal: string
  sisa_hari: number
  status: Status
  biaya: number | null
}

export interface Laporan {
  klien: { nama: string }
  penyusun: { nama: string }
  bulan: string // YYYY-MM
  label_bulan: string // "November 2026"
  dibuat: string // RFC 3339
  /** The owner's summary for this month; '' when none was written. */
  catatan: string
  ringkasan: {
    jumlah_aset: number
    per_status: Record<Status, number>
    pekerjaan: number
    menit: number
    biaya_30_hari: number
  }
  pekerjaan: BarisPekerjaan[]
  perpanjangan: BarisPerpanjangan[]
  aset: AsetLaporan[]
  jatuh_tempo: JatuhTempo[]
}

export type Layanan = 'slack' | 'discord' | 'telegram'

export interface Notifikasi {
  aktif: boolean
  layanan: Layanan | null
  /** The destination with its secret part masked, e.g. "discord.com/…6789"; null when off. */
  tujuan: string | null
  /** The Jakarta date of the last daily check (also set on days with nothing to send); null before the first. */
  terakhir: string | null
}

export const kunciLaporan = {
  semua: ['laporan'] as const,
  klien: (klienId: string) => ['laporan', klienId] as const,
  bulan: (klienId: string, bulan: string) => ['laporan', klienId, bulan] as const,
  /** The share path made in this session; the API shows it only once. */
  jalur: (klienId: string) => ['laporan', 'jalur', klienId] as const,
  publik: (token: string, bulan: string) => ['laporan', 'publik', token, bulan] as const,
}

export const KUNCI_NOTIFIKASI = ['notifikasi'] as const

const qsBulan = (bulan?: string) => (bulan ? `?bulan=${encodeURIComponent(bulan)}` : '')

export const laporanApi = {
  ambil: (klienId: string, bulan?: string) =>
    request<Laporan>(`/api/klien/${encodeURIComponent(klienId)}/laporan${qsBulan(bulan)}`),
  simpanCatatan: (klienId: string, bulan: string, teks: string) =>
    request<unknown>(`/api/klien/${encodeURIComponent(klienId)}/laporan/catatan`, {
      method: 'PUT',
      body: JSON.stringify({ bulan, teks }),
    }),
  /** Makes a new read-only link and drops the old one. Returns the path, e.g. /laporan/<token>. */
  buatTautan: (klienId: string) =>
    request<{ jalur: string }>(`/api/klien/${encodeURIComponent(klienId)}/laporan/tautan`, { method: 'POST' }).then(
      (r) => r.jalur,
    ),
  cabutTautan: (klienId: string) =>
    request<void>(`/api/klien/${encodeURIComponent(klienId)}/laporan/tautan`, { method: 'DELETE' }),
  /** PUBLIC: the report behind a share link. */
  publik: (token: string, bulan?: string) => request<Laporan>(`/api/laporan/${encodeURIComponent(token)}${qsBulan(bulan)}`),
}

export const notifikasiApi = {
  ambil: () => request<Notifikasi>('/api/notifikasi'),
  simpan: (webhook_url: string) =>
    request<Notifikasi>('/api/notifikasi', { method: 'PUT', body: JSON.stringify({ webhook_url }) }),
  matikan: () => request<void>('/api/notifikasi', { method: 'DELETE' }),
  uji: () => request<{ pesan?: string }>('/api/notifikasi/uji', { method: 'POST' }),
}

// ---------- months (Asia/Jakarta) ----------

const fmtBulanJakarta = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Jakarta', year: 'numeric', month: '2-digit' })
const fmtLabelBulan = new Intl.DateTimeFormat('id-ID', { month: 'long', year: 'numeric' })

/** The current month in Jakarta, "2026-11". */
export function bulanIni(now = new Date()): string {
  const p = Object.fromEntries(fmtBulanJakarta.formatToParts(now).map((x) => [x.type, x.value]))
  return `${p.year}-${p.month}`
}

/** "2026-11" moved by n months. */
export function geserBulan(bulan: string, n: number): string {
  const [y, m] = bulan.split('-').map(Number)
  const d = new Date(y, m - 1 + n, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

/** "November 2026" */
export function labelBulan(bulan: string): string {
  const [y, m] = bulan.split('-').map(Number)
  return fmtLabelBulan.format(new Date(y, m - 1, 1))
}

export const bulanValid = (s: string | null | undefined): s is string => !!s && /^\d{4}-(0[1-9]|1[0-2])$/.test(s)

/** The months a picker offers: this month and the eleven before it, newest first. */
export function daftarBulan(sampai = bulanIni(), jumlah = 12): string[] {
  return Array.from({ length: jumlah }, (_, i) => geserBulan(sampai, -i))
}
