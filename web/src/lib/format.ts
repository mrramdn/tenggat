// Indonesian formatting helpers shared by every screen.
import type { Jenis, Status } from './api'

export const labelStatus: Record<Status, string> = {
  lewat: 'Lewat',
  kritis: '7 hari',
  segera: '30 hari',
  aman: 'Aman',
}

export const keteranganStatus: Record<Status, string> = {
  lewat: 'Sudah berakhir',
  kritis: 'Berakhir hari ini sampai 7 hari lagi',
  segera: 'Berakhir 8 sampai 30 hari lagi',
  aman: 'Berakhir lebih dari 30 hari lagi',
}

export const labelJenis: Record<Jenis, string> = {
  sertifikat: 'Sertifikat TLS',
  domain: 'Domain',
  token: 'Token API',
  langganan: 'Langganan',
  lainnya: 'Lainnya',
}

export const urutanStatus: Status[] = ['lewat', 'kritis', 'segera', 'aman']

/** A calendar date "2026-11-13" as a local date, without time-zone shifts. */
export function keTanggal(iso: string): Date {
  const [y, m, d] = iso.slice(0, 10).split('-').map(Number)
  return new Date(y, m - 1, d)
}

const fmtTanggal = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })
const fmtTanggalPanjang = new Intl.DateTimeFormat('id-ID', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })
const fmtHari = new Intl.DateTimeFormat('id-ID', { weekday: 'long' })
const fmtRupiah = new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 })
const fmtRelatif = new Intl.RelativeTimeFormat('id-ID', { numeric: 'auto' })

/** "13 Nov 2026" */
export const tanggal = (iso: string) => fmtTanggal.format(keTanggal(iso))
/** "Jumat, 13 November 2026" */
export const tanggalPanjang = (iso: string) => fmtTanggalPanjang.format(keTanggal(iso))
/** "Jumat" */
export const hari = (iso: string) => fmtHari.format(keTanggal(iso))
/** "Rp 285.000" */
export const rupiah = (n: number) => fmtRupiah.format(n)

/** "Lewat 3 hari", "Hari ini", "Besok", "4 hari lagi" */
export function sisaHari(n: number): string {
  if (n < 0) return `Lewat ${-n} hari`
  if (n === 0) return 'Hari ini'
  if (n === 1) return 'Besok'
  return `${n} hari lagi`
}

/** "2 jam yang lalu" from an ISO timestamp. */
export function relatif(iso: string, now = Date.now()): string {
  const detik = Math.round((new Date(iso).getTime() - now) / 1000)
  const abs = Math.abs(detik)
  if (abs < 60) return 'baru saja'
  if (abs < 3600) return fmtRelatif.format(Math.round(detik / 60), 'minute')
  if (abs < 86400) return fmtRelatif.format(Math.round(detik / 3600), 'hour')
  return fmtRelatif.format(Math.round(detik / 86400), 'day')
}

/** CSS class suffix of each status, as used by .pill, .dot and the timeline bands. */
export const kelasStatus: Record<Status, 'lewat' | 'd7' | 'd30' | 'aman'> = {
  lewat: 'lewat',
  kritis: 'd7',
  segera: 'd30',
  aman: 'aman',
}

/** Status from days left, the same rule the API uses. */
export function statusDariSisa(n: number): Status {
  if (n < 0) return 'lewat'
  if (n <= 7) return 'kritis'
  if (n <= 30) return 'segera'
  return 'aman'
}

/** Short type names for tight places (phone sub-line, sheet on the phone). */
export const labelJenisPendek: Record<Jenis, string> = {
  sertifikat: 'TLS',
  domain: 'Domain',
  token: 'Token',
  langganan: 'Langganan',
  lainnya: 'Lainnya',
}

/** Lane names in the timeline. */
export const labelLajur: Record<Jenis, string> = {
  domain: 'Domain',
  sertifikat: 'TLS',
  token: 'Token API',
  langganan: 'Langganan',
  lainnya: 'Lainnya',
}

export const urutanJenis: Jenis[] = ['domain', 'sertifikat', 'token', 'langganan', 'lainnya']

const fmtTanggalPendek = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short' })
const fmtBulan = new Intl.DateTimeFormat('id-ID', { month: 'short' })
const fmtBulanTahun = new Intl.DateTimeFormat('id-ID', { month: 'short', year: 'numeric' })
const fmtAngka = new Intl.NumberFormat('id-ID')

/** "6 Nov" */
export const tanggalPendek = (iso: string) => fmtTanggalPendek.format(keTanggal(iso))
/** "Nov" */
export const bulan = (iso: string) => fmtBulan.format(keTanggal(iso))
/** "Nov 2026" */
export const bulanTahun = (iso: string) => fmtBulanTahun.format(keTanggal(iso))
/** "474.000" */
export const angka = (n: number) => fmtAngka.format(n)
/** "Rp 474.000" with a normal space, so it wraps like the mockup. */
export const rupiahBiasa = (n: number) => `Rp ${fmtAngka.format(n)}`

/** Unit under the big day number: "hari lagi" or "hari lewat". */
export const satuanHari = (n: number) => (n < 0 ? 'hari lewat' : 'hari lagi')

/** "19 hari lagi", "Lewat 3 hari", "Hari ini" */
export function frasaHari(n: number): string {
  if (n < 0) return `Lewat ${-n} hari`
  if (n === 0) return 'Berakhir hari ini'
  return `${n} hari lagi`
}

/** "baru saja", "12 menit lalu", "2 jam lalu", "3 hari lalu" */
export function lalu(iso: string, now = Date.now()): string {
  const detik = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (detik < 60) return 'baru saja'
  if (detik < 3600) return `${Math.floor(detik / 60)} menit lalu`
  if (detik < 86400) return `${Math.floor(detik / 3600)} jam lalu`
  return `${Math.floor(detik / 86400)} hari lalu`
}

/** "a", "a dan b", "a, b, dan c" */
export function gabung(daftar: string[]): string {
  if (daftar.length <= 1) return daftar.join('')
  if (daftar.length === 2) return `${daftar[0]} dan ${daftar[1]}`
  return `${daftar.slice(0, -1).join(', ')}, dan ${daftar[daftar.length - 1]}`
}

/** Today as YYYY-MM-DD in local time. */
export function hariIniIso(now = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`
}
