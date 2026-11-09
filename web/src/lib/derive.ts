// Numbers and sentences computed from the item list. Every screen reads from
// here, so the headline, the cards, the timeline and the list always agree.
import type { Item, Jenis, Status, Sumber } from './api'
import { bulan, bulanTahun, gabung, keTanggal, rupiahBiasa, tanggal, tanggalPendek } from './format'

export interface Saring {
  cari: string
  status: Status | null
  /** id of the chosen client */
  klien: string | null
  jenis: Jenis | null
  sumber: Sumber | null
  naik: boolean
}

export const saringAwal: Saring = { cari: '', status: null, klien: null, jenis: null, sumber: null, naik: true }

export const adaSaring = (s: Saring) => !!(s.cari.trim() || s.status || s.klien || s.jenis || s.sumber)

function cocok(it: Item, s: Saring, kecuali?: keyof Saring): boolean {
  if (kecuali !== 'status' && s.status && it.status !== s.status) return false
  if (kecuali !== 'klien' && s.klien !== null && it.klien_id !== s.klien) return false
  if (kecuali !== 'jenis' && s.jenis && it.jenis !== s.jenis) return false
  if (kecuali !== 'sumber' && s.sumber && it.sumber !== s.sumber) return false
  const q = s.cari.trim().toLowerCase()
  if (q) {
    const teks = `${it.nama} ${it.host ?? ''} ${it.klien}`.toLowerCase()
    if (!q.split(/\s+/).every((kata) => teks.includes(kata))) return false
  }
  return true
}

/** The list after every filter, sorted by days left. */
export function saringItem(items: Item[], s: Saring): Item[] {
  const hasil = items.filter((it) => cocok(it, s))
  return hasil.sort((a, b) => (s.naik ? a.sisa_hari - b.sisa_hari : b.sisa_hari - a.sisa_hari) || a.nama.localeCompare(b.nama, 'id'))
}

/** Items that pass every filter except one, for the counts next to that filter. */
export const tanpaSaring = (items: Item[], s: Saring, kecuali: keyof Saring) => items.filter((it) => cocok(it, s, kecuali))

export function hitungStatus(items: Item[]): Record<Status, number> {
  const n: Record<Status, number> = { lewat: 0, kritis: 0, segera: 0, aman: 0 }
  for (const it of items) n[it.status]++
  return n
}

/** Date range for a status card: "6 Nov", "13–15 Nov", "28 Nov–1 Des", "Jan–Mei". */
export function rentangTanggal(items: Item[], status: Status): string {
  const tgl = [...new Set(items.filter((it) => it.status === status).map((it) => it.kedaluwarsa))].sort()
  if (tgl.length === 0) return 'Tidak ada'
  const a = tgl[0]
  const b = tgl[tgl.length - 1]
  if (a === b) return tanggalPendek(a)
  const da = keTanggal(a)
  const db = keTanggal(b)
  if (da.getFullYear() === db.getFullYear() && da.getMonth() === db.getMonth()) return `${da.getDate()}–${tanggalPendek(b)}`
  if (status === 'aman') {
    const bulanan = (db.getTime() - da.getTime()) / 86_400_000 > 330
    return bulanan ? `${bulanTahun(a)}–${bulanTahun(b)}` : `${bulan(a)}–${bulan(b)}`
  }
  return `${tanggalPendek(a)}–${tanggalPendek(b)}`
}

/** Two-tone headline: a dark sentence, then a grey one. */
export function judul(items: Item[], hariIni = new Date()): { utama: string; sunyi: string | null } {
  const n = hitungStatus(items)
  // "minggu ini" only when every item in the 7-day band ends by this Sunday.
  const keMinggu = (7 - hariIni.getDay()) % 7
  const kritis = items.filter((it) => it.status === 'kritis')
  const kapan = kritis.every((it) => it.sisa_hari <= keMinggu) ? 'minggu ini' : 'dalam 7 hari'

  if (n.kritis > 0) {
    const utama = `${n.kritis} aset berakhir ${kapan}.`
    if (n.lewat > 0) return { utama, sunyi: `${n.lewat} sudah lewat.` }
    if (n.segera > 0) return { utama, sunyi: `${n.segera} lagi dalam 30 hari.` }
    if (n.aman > 0) return { utama, sunyi: 'Sisanya aman.' }
    return { utama, sunyi: null }
  }
  if (n.lewat > 0) {
    const utama = `${n.lewat} aset sudah lewat.`
    if (n.segera > 0) return { utama, sunyi: `${n.segera} berakhir dalam 30 hari.` }
    return { utama, sunyi: 'Tidak ada yang berakhir minggu ini.' }
  }
  if (n.segera > 0) return { utama: 'Tidak ada yang berakhir minggu ini.', sunyi: `${n.segera} aset berakhir dalam 30 hari.` }
  const terdekat = [...items].sort((a, b) => a.sisa_hari - b.sisa_hari)[0]
  return { utama: 'Semua aset aman.', sunyi: terdekat ? `Jatuh tempo terdekat ${tanggal(terdekat.kedaluwarsa)}.` : null }
}

/** Name as it reads inside a sentence: "domain tokokuemawar.id", "VPS 2 vCPU". */
export function namaDalamKalimat(it: Item): string {
  const m = /^(Domain|Token|Langganan)\s+(.+)$/.exec(it.nama)
  if (!m) return it.nama
  if (m[1] === 'Langganan') return m[2]
  return `${m[1].toLowerCase()} ${m[2]}`
}

/** Short label for a point on the timeline. */
export function labelTitik(it: Item): string {
  let s = it.jenis === 'sertifikat' && it.host ? it.host : it.nama.replace(/^(Domain|Token|Langganan)\s+/, '')
  if (s.length > 24) s = `${s.slice(0, 23).trimEnd()}…`
  return s
}

/** Renewal cost of everything that ends within 30 days, overdue included. */
export function biaya30(items: Item[]): { total: number; nama: string[] } {
  const kena = items.filter((it) => it.sisa_hari <= 30 && (it.biaya ?? 0) > 0).sort((a, b) => a.sisa_hari - b.sisa_hari)
  return { total: kena.reduce((t, it) => t + (it.biaya ?? 0), 0), nama: kena.map(namaDalamKalimat) }
}

export function kalimatBiaya(items: Item[]): { pre: string; jumlah: string | null; post: string } {
  const b = biaya30(items)
  if (b.total === 0) return { pre: 'Tidak ada biaya perpanjangan dalam 30 hari ke depan.', jumlah: null, post: '' }
  const dari = b.nama.length > 3 ? `${b.nama.length} aset` : gabung(b.nama)
  return { pre: 'Biaya perpanjangan 30 hari ke depan ', jumlah: rupiahBiasa(b.total), post: `, untuk ${dari}.` }
}

const BOBOT: Record<Status, number> = { lewat: 0, kritis: 1, segera: 2, aman: 3 }

/** The most urgent status of each client's assets, by client id (the dot in the sidebar). */
export function statusTerburuk(items: Item[]): Map<string, Status> {
  const m = new Map<string, Status>()
  for (const it of items) {
    if (!it.klien_id) continue
    const ada = m.get(it.klien_id)
    if (!ada || BOBOT[it.status] < BOBOT[ada]) m.set(it.klien_id, it.status)
  }
  return m
}
