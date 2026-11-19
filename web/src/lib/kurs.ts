// Daily reference exchange rates (GET /api/kurs) and display-only rupiah estimates.
import { useQuery } from '@tanstack/react-query'
import { request, type BiayaPerMataUang, type MataUang } from './api'
import { normalisasiMataUang, tanggal, uang, urutanMataUang } from './format'

export interface Kurs {
  tanggal: string | null
  per_idr: Partial<Record<MataUang, number>> | null
  sumber: string
}

export const KUNCI_KURS = ['kurs'] as const

const SATU_JAM = 60 * 60 * 1000

export const kursApi = {
  ambil: () => request<Kurs>('/api/kurs'),
}

/** Shared query for GET /api/kurs: stale for an hour, no retries, never blocks rendering. */
export function useKurs(): Kurs | null {
  const q = useQuery({
    queryKey: KUNCI_KURS,
    queryFn: kursApi.ambil,
    staleTime: SATU_JAM,
    gcTime: SATU_JAM,
    retry: false,
    refetchOnWindowFocus: false,
  })
  return q.data ?? null
}

/** Converts an amount + currency into a whole-rupiah estimate using `kurs`, or null when rates are missing. */
export function kiraRupiah(
  jumlah: number | null | undefined,
  mataUang: MataUang | null | undefined,
  kurs: Kurs | null | undefined,
): number | null {
  if (jumlah == null || !Number.isFinite(jumlah) || jumlah <= 0) return null
  const mu = normalisasiMataUang(mataUang)
  if (mu === 'IDR') return Math.round(jumlah)
  const rate = kurs?.per_idr?.[mu]
  if (rate == null || !Number.isFinite(rate) || rate <= 0) return null
  return Math.round(jumlah * rate)
}

/** Quiet rupiah estimate for a single foreign-currency amount ("≈ Rp 1.834.527"), or null for IDR / missing rate. */
export function teksKiraRupiah(
  jumlah: number | null | undefined,
  mataUang: MataUang | null | undefined,
  kurs: Kurs | null | undefined,
): string | null {
  const mu = normalisasiMataUang(mataUang)
  if (mu === 'IDR') return null
  const rp = kiraRupiah(jumlah, mu, kurs)
  return rp != null ? `≈ ${uang(rp, 'IDR')}` : null
}

/** True when a per-currency total has more than one currency or its only currency is foreign. */
export function perluKiraTotal(perMataUang: BiayaPerMataUang | null | undefined): boolean {
  if (!perMataUang) return false
  let jumlah = 0
  let adaAsing = false
  for (const mu of urutanMataUang) {
    const v = perMataUang[mu]
    if (v != null && v > 0) {
      jumlah++
      if (mu !== 'IDR') adaAsing = true
    }
  }
  return jumlah > 1 || adaAsing
}

/** Combined whole-rupiah estimate for a per-currency map; null for IDR-only maps or when any needed rate is missing. */
export function kiraTotalRupiah(
  perMataUang: BiayaPerMataUang | null | undefined,
  kurs: Kurs | null | undefined,
): number | null {
  if (!perMataUang || !perluKiraTotal(perMataUang)) return null
  let total = 0
  for (const mu of urutanMataUang) {
    const v = perMataUang[mu]
    if (v == null || v <= 0) continue
    const rp = kiraRupiah(v, mu, kurs)
    if (rp == null) return null
    total += rp
  }
  return total
}

/** Reference-rate disclaimer shown once per screen (tooltip on desktop, visible text under the dashboard sentence). */
export function catatanKurs(kurs: Kurs | null | undefined): string | undefined {
  if (!kurs?.tanggal) return undefined
  return `Perkiraan dengan kurs referensi ECB ${tanggal(kurs.tanggal)}, bukan kurs bank Anda.`
}
