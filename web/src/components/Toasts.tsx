// Short confirmations at the bottom of the screen (saved, deleted, checked...).
import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { ApiError } from '../lib/api'
import { Icon } from './Icon'

type Jenis = 'ok' | 'error'
interface Toast {
  id: number
  jenis: Jenis
  judul: string
  teks?: string
}

interface ToastApi {
  ok: (judul: string, teks?: string) => void
  galat: (judul: string, teks?: string) => void
  /** Toast for a failed request, with a special sentence for 429. */
  dariGalat: (judul: string, err: unknown) => void
}

const Ctx = createContext<ToastApi | null>(null)

export function useToast(): ToastApi {
  const v = useContext(Ctx)
  if (!v) throw new Error('useToast di luar ToastProvider')
  return v
}

export function pesanGalat(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 429) {
      return err.retryAfter ? `Terlalu banyak permintaan. Coba lagi dalam ${err.retryAfter} detik.` : err.message
    }
    return err.message
  }
  return 'Terjadi kesalahan.'
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [list, setList] = useState<Toast[]>([])
  const next = useRef(1)

  const push = useCallback((jenis: Jenis, judul: string, teks?: string) => {
    const id = next.current++
    setList((l) => [...l.slice(-2), { id, jenis, judul, teks }])
    window.setTimeout(() => setList((l) => l.filter((t) => t.id !== id)), jenis === 'error' ? 7000 : 4000)
  }, [])

  const api = useMemo<ToastApi>(
    () => ({
      ok: (j, t) => push('ok', j, t),
      galat: (j, t) => push('error', j, t),
      dariGalat: (j, err) => push('error', j, pesanGalat(err)),
    }),
    [push],
  )

  return (
    <Ctx.Provider value={api}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {list.map((t) => (
          <div key={t.id} className={`toast is-${t.jenis}`}>
            <Icon name={t.jenis === 'ok' ? 'check' : 'alert'} />
            <div>
              <b>{t.judul}</b>
              {t.teks ? <span> {t.teks}</span> : null}
            </div>
          </div>
        ))}
      </div>
    </Ctx.Provider>
  )
}
