// The public lookup behind the demo on the front page: a domain's expiry from its registry (RDAP) and the
// certificate's expiry from the server, read without a session and without saving anything.
import { request, type Status } from './api'

export interface CekDomain {
  /** The registrable domain, e.g. github.com for www.github.com. */
  domain: string
  kedaluwarsa: string // YYYY-MM-DD
  sisa_hari: number
  status: Status
  registrar: string
}

export interface CekSertifikat {
  kedaluwarsa: string // YYYY-MM-DD
  sisa_hari: number
  status: Status
  penerbit: string
}

/** Each half can fail on its own: .io has no RDAP, a parked domain may have no TLS. */
export interface HasilCek {
  host: string
  domain: CekDomain | null
  domain_galat: string | null
  sertifikat: CekSertifikat | null
  sertifikat_galat: string | null
}

export const publik = {
  /** 422 when the host is not valid, 429 (with Retry-After) past the public rate limit. */
  cek: (host: string) => request<HasilCek>(`/api/publik/cek?host=${encodeURIComponent(host)}`),
}

/** "https://www.Toko.id/kontak" → "www.toko.id": what people paste is often a URL. */
export function rapikanHost(isian: string): string {
  let h = isian.trim().toLowerCase()
  h = h.replace(/^[a-z][a-z0-9+.-]*:\/\//, '')
  h = h.replace(/[/?#].*$/, '')
  h = h.replace(/^[^@]*@/, '')
  return h.replace(/\.$/, '')
}
