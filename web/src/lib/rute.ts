// A small router on the History API: the path decides the screen, links push a new entry, and the back button
// works. No dependency; every screen reads the current route with useRute().
import { createElement, useSyncExternalStore, type AnchorHTMLAttributes, type MouseEvent, type ReactElement } from 'react'

export type TabKlien = 'aset' | 'tugas' | 'aktivitas' | 'laporan'

export type Rute =
  | { nama: 'beranda' }
  | { nama: 'masuk' }
  | { nama: 'daftar' }
  | { nama: 'ringkasan' }
  | { nama: 'klien'; id: string; tab: TabKlien }
  | { nama: 'pengaturan' }
  | { nama: 'laporan-publik'; token: string }
  | { nama: 'tidak-ditemukan' }

export const TAB_KLIEN: TabKlien[] = ['aset', 'tugas', 'aktivitas', 'laporan']

const ID = /^[A-Za-z0-9_-]{1,64}$/

/** Reads a path such as /app/klien/abc/tugas. A trailing slash is ignored. */
export function bacaRute(path: string): Rute {
  const p = path.replace(/\/+$/, '') || '/'
  if (p === '/') return { nama: 'beranda' }
  if (p === '/masuk') return { nama: 'masuk' }
  if (p === '/daftar') return { nama: 'daftar' }
  if (p === '/app') return { nama: 'ringkasan' }
  if (p === '/app/pengaturan') return { nama: 'pengaturan' }
  const k = /^\/app\/klien\/([^/]+)(?:\/([^/]+))?$/.exec(p)
  if (k && ID.test(k[1])) {
    const tab = (k[2] ?? 'aset') as TabKlien
    if (TAB_KLIEN.includes(tab)) return { nama: 'klien', id: k[1], tab }
  }
  const l = /^\/laporan\/([^/]+)$/.exec(p)
  if (l && ID.test(l[1])) return { nama: 'laporan-publik', token: l[1] }
  return { nama: 'tidak-ditemukan' }
}

/** True for the routes that need a session (everything under /app). */
export const perluMasuk = (r: Rute) => r.nama === 'ringkasan' || r.nama === 'klien' || r.nama === 'pengaturan'

/** The path of a client's space, with its tab. */
export const jalurKlien = (id: string, tab: TabKlien = 'aset') => (tab === 'aset' ? `/app/klien/${id}` : `/app/klien/${id}/${tab}`)

const pendengar = new Set<() => void>()
function beritahu() {
  for (const f of pendengar) f()
}
function langganan(f: () => void) {
  pendengar.add(f)
  window.addEventListener('popstate', f)
  return () => {
    pendengar.delete(f)
    window.removeEventListener('popstate', f)
  }
}

let terakhir = { kunci: '', rute: bacaRute('/') }
function rutekini(): Rute {
  const kunci = window.location.pathname
  if (kunci !== terakhir.kunci) terakhir = { kunci, rute: bacaRute(kunci) }
  return terakhir.rute
}

/** The current route; the component renders again when it changes. */
export function useRute(): Rute {
  return useSyncExternalStore(langganan, rutekini)
}

/** The current path and query, e.g. /app/klien/abc?x=1. */
export const lokasiKini = () => window.location.pathname + window.location.search

/** Goes to a path. ganti: replace the current history entry (redirects) instead of adding one. */
export function pergi(path: string, opsi: { ganti?: boolean } = {}): void {
  if (path === lokasiKini()) return
  const pindahHalaman = new URL(path, window.location.origin).pathname !== window.location.pathname
  if (opsi.ganti) window.history.replaceState(null, '', path)
  else window.history.pushState(null, '', path)
  if (pindahHalaman && !opsi.ganti) window.scrollTo({ top: 0 })
  beritahu()
}

/** Where to go after signing in: ?lanjut= when it is a path inside the app, else /app. */
export function tujuanLanjut(): string {
  const l = new URLSearchParams(window.location.search).get('lanjut')
  if (l && l.startsWith('/app') && !l.startsWith('//')) return l
  return '/app'
}

/** The sign-in path that comes back to the current page afterwards. */
export const jalurMasuk = () => `/masuk?lanjut=${encodeURIComponent(lokasiKini())}`

/** A link inside the app: a real <a href> (open in a new tab works), but a plain click changes the route in place. */
export function Tautan({ ke, onClick, ...props }: AnchorHTMLAttributes<HTMLAnchorElement> & { ke: string }): ReactElement {
  function klik(e: MouseEvent<HTMLAnchorElement>) {
    onClick?.(e)
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
    if (props.target && props.target !== '_self') return
    e.preventDefault()
    pergi(ke)
  }
  return createElement('a', { ...props, href: ke, onClick: klik })
}
