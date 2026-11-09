// Typed client for the Tenggat API. Every request goes to the same origin; nginx
// (and the Vite dev server) forwards /api to the Go service.

export type Jenis = 'sertifikat' | 'domain' | 'token' | 'langganan' | 'lainnya'
export type Status = 'lewat' | 'kritis' | 'segera' | 'aman'
export type Sumber = 'manual' | 'server' | 'registri'

export interface TLSInfo {
  penerbit: string
  subjek: string
  alamat?: string
  berlaku_sejak: string
  berakhir: string
  rantai_valid: boolean
  galat_rantai?: string
}

export interface Item {
  id: string
  nama: string
  jenis: Jenis
  klien: string // the client's name, '' without a client
  klien_id: string | null
  host: string | null
  kedaluwarsa: string // YYYY-MM-DD
  sisa_hari: number
  status: Status
  sumber: Sumber
  biaya: number | null
  catatan: string
  tls: TLSInfo | null
  registri: RegistriInfo | null
  terakhir_dicek: string | null
  galat_cek: string | null
  dibuat: string
  diubah: string
}

export interface Ringkasan {
  lewat: number
  kritis: number
  segera: number
  aman: number
  total: number
  klien: string[]
}

export interface ItemInput {
  nama?: string
  jenis: Jenis
  /** A client's name: an unknown name becomes a new client, '' means no client. Ignored when klien_id is set. */
  klien?: string
  klien_id?: string
  host?: string
  kedaluwarsa?: string
  biaya?: number | null
  catatan?: string
}

export interface PratinjauSertifikat {
  host: string
  kedaluwarsa: string
  sisa_hari: number
  status: Status
  tls: TLSInfo
}

/** What the domain registry said at the last lookup (RDAP). */
export interface RegistriInfo {
  domain: string
  registrar: string
  server: string
}

export interface PratinjauDomain {
  host: string
  /** The registrable domain, e.g. github.com for www.github.com. */
  domain: string
  kedaluwarsa: string
  sisa_hari: number
  status: Status
  registri: { registrar: string; server: string }
}

export interface Filter {
  status?: Status
  klien?: string
}

/** An error answer from the API, with per-field messages for 422. */
export class ApiError extends Error {
  status: number
  field: Record<string, string>
  /** Seconds to wait, from the Retry-After header of a 429 answer. */
  retryAfter: number | null

  constructor(status: number, message: string, field: Record<string, string> = {}, retryAfter: number | null = null) {
    super(message)
    this.status = status
    this.field = field
    this.retryAfter = retryAfter
  }
}

export interface Pengguna {
  id: string
  nama: string
  email: string
  dibuat: string
}

export interface DaftarInput {
  nama: string
  email: string
  kata_sandi: string
}

export interface MasukInput {
  email: string
  kata_sandi: string
}

// Called once for a 401 from any request except the auth endpoints: the session
// cookie expired or was revoked. App registers it and goes back to the sign-in screen.
let sesiBerakhir: (() => void) | null = null
export function saatSesiBerakhir(fn: (() => void) | null) {
  sesiBerakhir = fn
}

export async function kirim(path: string, init?: RequestInit): Promise<Response> {
  try {
    return await fetch(path, {
      ...init,
      headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
    })
  } catch {
    throw new ApiError(0, 'Tidak dapat terhubung ke server. Periksa koneksi Anda.')
  }
}

/** Turns a failed response into an ApiError. auth: the request is part of signing in or out, so a 401 is an
 *  answer, not an expired session. */
export async function gagal(res: Response, auth: boolean): Promise<never> {
  const body = await res.json().catch(() => ({}))
  if (res.status === 401 && !auth) sesiBerakhir?.()
  const retry = Number(res.headers.get('Retry-After'))
  throw new ApiError(
    res.status,
    body.galat ?? 'Terjadi kesalahan.',
    body.field ?? {},
    Number.isFinite(retry) && retry > 0 ? retry : null,
  )
}

export async function request<T>(path: string, init?: RequestInit, auth = false): Promise<T> {
  const res = await kirim(path, init)
  if (res.status === 204) return undefined as T
  if (!res.ok) return gagal(res, auth)
  return (await res.json().catch(() => ({}))) as T
}

/** Saves a file from a GET answer. Through fetch rather than a plain link, so an expired session goes back to the
 *  sign-in screen instead of saving the error as the file. The name comes from Content-Disposition when present. */
export async function unduh(path: string, nama: string): Promise<void> {
  const res = await kirim(path)
  if (!res.ok) return gagal(res, false)
  const dariServer = /filename="?([^";]+)"?/.exec(res.headers.get('Content-Disposition') ?? '')?.[1]
  const url = URL.createObjectURL(await res.blob())
  const a = document.createElement('a')
  a.href = url
  a.download = dariServer ?? nama
  a.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 10_000)
}

export const api = {
  daftar(filter: Filter = {}): Promise<Item[]> {
    const q = new URLSearchParams()
    if (filter.status) q.set('status', filter.status)
    if (filter.klien) q.set('klien', filter.klien)
    const qs = q.toString()
    return request<{ item: Item[] }>(`/api/item${qs ? `?${qs}` : ''}`).then((r) => r.item)
  },
  ringkasan: () => request<Ringkasan>('/api/ringkasan'),
  ambil: (id: string) => request<Item>(`/api/item/${id}`),
  buat: (input: ItemInput) => request<Item>('/api/item', { method: 'POST', body: JSON.stringify(input) }),
  ubah: (id: string, input: ItemInput) =>
    request<Item>(`/api/item/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  hapus: (id: string) => request<void>(`/api/item/${id}`, { method: 'DELETE' }),
  cekUlang: (id: string) => request<Item>(`/api/item/${id}/cek`, { method: 'POST' }),
  pratinjau: (host: string) =>
    request<PratinjauSertifikat>(`/api/sertifikat?host=${encodeURIComponent(host)}`),
  kalenderUrl: '/api/kalender.ics',
  /** Saves the calendar as tenggat.ics. Through fetch rather than a plain link, so an expired session goes back to
   *  the sign-in screen instead of saving the error as the file. */
  async unduhKalender(): Promise<void> {
    const res = await kirim('/api/kalender.ics')
    if (!res.ok) return gagal(res, false)
    const url = URL.createObjectURL(await res.blob())
    const a = document.createElement('a')
    a.href = url
    a.download = 'tenggat.ics'
    a.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 10_000)
  },
  /** Reads a domain's expiry date from its registry (RDAP) without saving anything. */
  pratinjauDomain: (host: string) => request<PratinjauDomain>(`/api/domain?host=${encodeURIComponent(host)}`),
  /** Whether the account has a calendar subscription link. The link itself is shown only when it is made. */
  kalenderAktif: () => request<{ kalender_aktif: boolean }>('/api/saya').then((r) => r.kalender_aktif),
  /** Makes a new subscription link and drops the old one. Returns the path, e.g. /api/kalender/<token>.ics. */
  buatTautanKalender: () => request<{ jalur: string }>('/api/kalender/tautan', { method: 'POST' }).then((r) => r.jalur),
  hapusTautanKalender: () => request<void>('/api/kalender/tautan', { method: 'DELETE' }),
  /** Saves every asset as tenggat-YYYY-MM-DD.csv. */
  unduhCsv: () => unduh('/api/item.csv', 'tenggat.csv'),
}

// Accounts. The session lives in the HttpOnly cookie tenggat_sesi, which the
// browser sends with every same-origin request; JavaScript never reads it.
export const auth = {
  /** The signed-in user, or null when there is no valid session. */
  saya: () =>
    request<{ pengguna: Pengguna }>('/api/saya', undefined, true).then(
      (r) => r.pengguna,
      (err: unknown) => {
        if (err instanceof ApiError && err.status === 401) return null
        throw err
      },
    ),
  daftar: (input: DaftarInput) =>
    request<{ pengguna: Pengguna }>('/api/akun', { method: 'POST', body: JSON.stringify(input) }, true).then((r) => r.pengguna),
  masuk: (input: MasukInput) =>
    request<{ pengguna: Pengguna }>('/api/sesi', { method: 'POST', body: JSON.stringify(input) }, true).then((r) => r.pengguna),
  keluar: () => request<void>('/api/sesi', { method: 'DELETE' }, true),
  hapusAkun: () => request<void>('/api/saya', { method: 'DELETE' }),
}

// Clients. Their figures (asset count, cost due) change whenever an asset changes, so every write of an asset or
// a client invalidates KUNCI_KLIEN as well as the item list.
export const KUNCI_KLIEN = ['klien'] as const

export interface Klien {
  id: string
  nama: string
  catatan: string
  jumlah_aset: number
  terdekat: string | null // YYYY-MM-DD, null without assets
  biaya_30_hari: number
}

export interface KlienInput {
  nama: string
  catatan: string
}

export const klienApi = {
  daftar: () => request<{ klien: Klien[] }>('/api/klien').then((r) => r.klien),
  buat: (input: KlienInput) => request<Klien>('/api/klien', { method: 'POST', body: JSON.stringify(input) }),
  ubah: (id: string, input: KlienInput) =>
    request<Klien>(`/api/klien/${id}`, { method: 'PUT', body: JSON.stringify(input) }),
  hapus: (id: string) => request<void>(`/api/klien/${id}`, { method: 'DELETE' }),
}
