// Personal access tokens for external tools (MCP / API). Managed with the session cookie only.
import { request } from './api'

export type CakupanToken = 'baca' | 'tulis'

export interface TokenAkses {
  id: string
  nama: string
  awalan: string
  cakupan: CakupanToken
  dibuat: string // RFC 3339
  terakhir_dipakai: string | null // RFC 3339
  kedaluwarsa: string | null // RFC 3339
}

export interface TokenInput {
  nama: string
  cakupan: CakupanToken
  /** Omit for the default 90 days, pass null for no expiry, or 1..365. */
  berlaku_hari?: number | null
}

export interface TokenBaru {
  token: TokenAkses
  rahasia: string
}

export const KUNCI_TOKEN = ['token'] as const

export const tokenApi = {
  daftar: () => request<{ token: TokenAkses[] }>('/api/token').then((r) => r.token ?? []),
  buat: (input: TokenInput) =>
    request<TokenBaru>('/api/token', { method: 'POST', body: JSON.stringify(input) }),
  cabut: (id: string) => request<void>(`/api/token/${id}`, { method: 'DELETE' }),
}
