// Recurring maintenance tasks and the client activity feed (notes, the work log and what the API records itself).
import { useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'
import { KUNCI_KLIEN, request, type Status } from './api'
import { hariIniIso, keTanggal } from './format'

export type SatuanUlang = 'hari' | 'minggu' | 'bulan' | 'tahun'

export interface Tugas {
  id: string
  klien_id: string | null
  /** The client's name, '' without a client. */
  klien: string
  judul: string
  catatan: string
  ulang_jumlah: number
  ulang_satuan: SatuanUlang
  jatuh_tempo: string // YYYY-MM-DD
  sisa_hari: number
  status: Status
  biaya: number | null
  terakhir_selesai: string | null // RFC 3339
  dibuat: string
  diubah: string
}

export interface TugasInput {
  klien_id?: string
  /** A client's name, as for assets. Ignored when klien_id is set. */
  klien?: string
  judul: string
  catatan?: string
  ulang_jumlah: number
  ulang_satuan: SatuanUlang
  jatuh_tempo: string
  biaya?: number | null
}

export interface SelesaiInput {
  catatan?: string
  durasi_menit?: number
  /** YYYY-MM-DD; the API takes today (Jakarta) without it. */
  tanggal?: string
}

export type JenisAktivitas =
  | 'catatan'
  | 'pekerjaan'
  | 'aset_baru'
  | 'aset_diperpanjang'
  | 'aset_dihapus'
  | 'cek_diperbarui'
  | 'cek_gagal'
  | 'tugas_baru'
  | 'tugas_selesai'
  | 'tugas_dihapus'
  | 'laporan_dibagikan'

export interface Aktivitas {
  id: string
  klien_id: string | null
  klien: string
  item_id: string | null
  tugas_id: string | null
  jenis: JenisAktivitas
  teks: string
  tanggal: string // YYYY-MM-DD, Jakarta
  durasi_menit: number | null
  data: Record<string, unknown> | null
  dibuat: string // RFC 3339
}

export interface HalamanAktivitas {
  aktivitas: Aktivitas[]
  lagi: boolean
}

export interface AktivitasInput {
  klien_id: string
  jenis: 'catatan' | 'pekerjaan'
  teks: string
  tanggal?: string
  durasi_menit?: number
}

/** Query keys. Every key starts with 'tugas' or 'aktivitas', so invalidating the first part reaches all of them. */
export const kunciTugas = {
  semua: ['tugas'] as const,
  klien: (klienId: string) => ['tugas', 'klien', klienId] as const,
  dalamHari: (n: number) => ['tugas', 'dalam', n] as const,
}
export const kunciAktivitas = {
  semua: ['aktivitas'] as const,
  klien: (klienId: string) => ['aktivitas', 'klien', klienId] as const,
}

export const BATAS_AKTIVITAS = 50

export const tugasApi = {
  daftar: (opsi: { klienId?: string; dalamHari?: number } = {}) => {
    const q = new URLSearchParams()
    if (opsi.klienId) q.set('klien_id', opsi.klienId)
    if (opsi.dalamHari !== undefined) q.set('dalam_hari', String(opsi.dalamHari))
    const qs = q.toString()
    return request<{ tugas: Tugas[] }>(`/api/tugas${qs ? `?${qs}` : ''}`).then((r) => r.tugas)
  },
  buat: (input: TugasInput) => request<Tugas>('/api/tugas', { method: 'POST', body: JSON.stringify(input) }),
  ubah: (id: string, input: TugasInput) =>
    request<Tugas>(`/api/tugas/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  hapus: (id: string) => request<void>(`/api/tugas/${id}`, { method: 'DELETE' }),
  selesai: (id: string, input: SelesaiInput = {}) =>
    request<Tugas>(`/api/tugas/${id}/selesai`, { method: 'POST', body: JSON.stringify(input) }),
}

export const aktivitasApi = {
  /** One page, newest first. sebelum: the id of the last entry already shown. */
  daftar: (opsi: { klienId: string; sebelum?: string; batas?: number }) => {
    const q = new URLSearchParams({ klien_id: opsi.klienId, batas: String(opsi.batas ?? BATAS_AKTIVITAS) })
    if (opsi.sebelum) q.set('sebelum', opsi.sebelum)
    return request<HalamanAktivitas>(`/api/aktivitas?${q}`).then((r) => ({
      aktivitas: r.aktivitas ?? [],
      lagi: !!r.lagi,
    }))
  },
  buat: (input: AktivitasInput) =>
    request<Aktivitas>('/api/aktivitas', { method: 'POST', body: JSON.stringify(input) }),
  hapus: (id: string) => request<void>(`/api/aktivitas/${id}`, { method: 'DELETE' }),
}

/** Refreshes everything a task or activity write can change: the task lists, the feeds, the client figures and
 *  the monthly reports (work entries and finished tasks are part of them). */
export function useSegarkan() {
  const qc = useQueryClient()
  return useCallback(() => {
    void qc.invalidateQueries({ queryKey: kunciTugas.semua })
    void qc.invalidateQueries({ queryKey: kunciAktivitas.semua })
    void qc.invalidateQueries({ queryKey: KUNCI_KLIEN })
    void qc.invalidateQueries({ queryKey: ['laporan'] })
  }, [qc])
}

// ---------- wording and dates ----------

const namaSatuan: Record<SatuanUlang, string> = { hari: 'hari', minggu: 'minggu', bulan: 'bulan', tahun: 'tahun' }

/** "Tiap bulan", "Tiap 3 bulan", "Tiap minggu" */
export function labelUlang(jumlah: number, satuan: SatuanUlang): string {
  return jumlah === 1 ? `Tiap ${namaSatuan[satuan]}` : `Tiap ${jumlah} ${namaSatuan[satuan]}`
}

/** "45 menit", "1 jam", "1 jam 30 menit" */
export function durasi(menit: number): string {
  const j = Math.floor(menit / 60)
  const m = menit % 60
  if (j === 0) return `${m} menit`
  return m === 0 ? `${j} jam` : `${j} jam ${m} menit`
}

const iso = (d: Date) => hariIniIso(d)

/** The date one interval after `dari`. Month steps keep the day where the month has it, else its last day. */
export function tambahInterval(dari: string, jumlah: number, satuan: SatuanUlang): string {
  const d = keTanggal(dari)
  if (satuan === 'hari') return iso(new Date(d.getFullYear(), d.getMonth(), d.getDate() + jumlah))
  if (satuan === 'minggu') return iso(new Date(d.getFullYear(), d.getMonth(), d.getDate() + jumlah * 7))
  const bulan = satuan === 'bulan' ? jumlah : jumlah * 12
  const target = new Date(d.getFullYear(), d.getMonth() + bulan, 1)
  const akhir = new Date(target.getFullYear(), target.getMonth() + 1, 0).getDate()
  return iso(new Date(target.getFullYear(), target.getMonth(), Math.min(d.getDate(), akhir)))
}

/** The due date after marking the task done: one interval past the old due date, repeated until after today.
 *  The API does the same; this is only for the sentence in the dialog. */
export function jatuhTempoBerikutnya(t: Pick<Tugas, 'jatuh_tempo' | 'ulang_jumlah' | 'ulang_satuan'>, hariIni = hariIniIso()): string {
  let k = 1
  let d = tambahInterval(t.jatuh_tempo, t.ulang_jumlah, t.ulang_satuan)
  while (d <= hariIni && k < 5000) d = tambahInterval(t.jatuh_tempo, ++k * t.ulang_jumlah, t.ulang_satuan)
  return d
}
