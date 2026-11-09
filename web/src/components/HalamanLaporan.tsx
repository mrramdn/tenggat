// The public report behind a share link (/laporan/:token?bulan=YYYY-MM): the document only, a quiet header with
// month navigation and a print button. No sign-in; search engines are asked not to index it.
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { ApiError } from '../lib/api'
import { bulanIni, bulanValid, geserBulan, kunciLaporan, labelBulan, laporanApi } from '../lib/api-laporan'
import { Icon } from './Icon'
import { IkonLaporan } from './IkonLaporan'
import { CetakLaporan, LaporanDokumen, cetak } from './LaporanDokumen'
import { pesanGalat } from './Toasts'

function bulanDariUrl(): string {
  const b = new URLSearchParams(window.location.search).get('bulan')
  return bulanValid(b) ? b : bulanIni()
}

function useNoindex() {
  useEffect(() => {
    const meta = document.createElement('meta')
    meta.name = 'robots'
    meta.content = 'noindex, nofollow'
    document.head.appendChild(meta)
    const ref = document.createElement('meta')
    ref.name = 'referrer'
    ref.content = 'no-referrer'
    document.head.appendChild(ref)
    return () => {
      meta.remove()
      ref.remove()
    }
  }, [])
}

export function HalamanLaporan({ token }: { token: string }) {
  useNoindex()
  const kini = bulanIni()
  const [bulan, setBulanState] = useState(bulanDariUrl)

  useEffect(() => {
    const ikut = () => setBulanState(bulanDariUrl())
    window.addEventListener('popstate', ikut)
    return () => window.removeEventListener('popstate', ikut)
  }, [])

  function setBulan(b: string) {
    const url = new URL(window.location.href)
    if (b === kini) url.searchParams.delete('bulan')
    else url.searchParams.set('bulan', b)
    window.history.pushState(window.history.state, '', url.pathname + url.search + url.hash)
    setBulanState(b)
  }

  const q = useQuery({
    queryKey: kunciLaporan.publik(token, bulan),
    queryFn: () => laporanApi.publik(token, bulan),
    placeholderData: (lama) => lama,
    retry: (n, err) => n < 1 && !(err instanceof ApiError && err.status >= 400 && err.status < 500),
  })

  const l = q.data
  const tidakAda = q.isError && q.error instanceof ApiError && q.error.status === 404

  useEffect(() => {
    const lama = document.title
    document.title = l ? `Laporan ${l.klien.nama} ${l.label_bulan}` : tidakAda ? 'Laporan tidak ditemukan' : 'Laporan'
    return () => {
      document.title = lama
    }
  }, [l, tidakAda])

  const bisaMaju = bulan < kini

  return (
    <div className="hl">
      <header className="hl__kepala">
        <a className="wordmark" href="/" aria-label="Tenggat, halaman depan">
          tenggat
        </a>
        {tidakAda ? null : (
          <div className="hl__alat">
            <nav className="hl__bulan" aria-label="Bulan laporan">
              <button
                type="button"
                className="btn btn--ghost btn--icon"
                aria-label={`Bulan sebelumnya, ${labelBulan(geserBulan(bulan, -1))}`}
                onClick={() => setBulan(geserBulan(bulan, -1))}
              >
                <IkonLaporan name="chevron-left" />
              </button>
              <span className="hl__label" aria-live="polite">
                {labelBulan(bulan)}
              </span>
              <button
                type="button"
                className="btn btn--ghost btn--icon"
                aria-label={bisaMaju ? `Bulan berikutnya, ${labelBulan(geserBulan(bulan, 1))}` : 'Bulan berikutnya'}
                onClick={() => setBulan(geserBulan(bulan, 1))}
                disabled={!bisaMaju}
              >
                <IkonLaporan name="chevron-right" />
              </button>
            </nav>
            <button
              type="button"
              className="btn btn--secondary hl__cetak"
              onClick={() => {
                if (l) cetak(l)
              }}
              disabled={!l || q.isPlaceholderData}
              aria-label="Cetak atau simpan sebagai PDF"
            >
              <IkonLaporan name="printer" />
              <span>Cetak / Simpan PDF</span>
            </button>
          </div>
        )}
      </header>

      <main className="hl__isi">
        {tidakAda ? (
          <div className="hl__pesan">
            <h1>Tautan laporan ini tidak berlaku lagi.</h1>
            <p>
              Pemilik laporan mungkin sudah mencabut atau mengganti tautannya. Mintalah tautan baru kepada orang yang
              mengirimkannya kepada Anda.
            </p>
          </div>
        ) : q.isError && !l ? (
          <div className="hl__pesan" role="alert">
            <h1>Laporan tidak dapat dimuat.</h1>
            <p>{pesanGalat(q.error)}</p>
            <button type="button" className="btn btn--secondary" onClick={() => void q.refetch()}>
              <Icon name="refresh" />
              Coba lagi
            </button>
          </div>
        ) : l ? (
          <div aria-busy={q.isFetching} style={q.isPlaceholderData ? { opacity: 0.55, transition: 'opacity 120ms' } : undefined}>
            <LaporanDokumen laporan={l} />
            {q.isPlaceholderData ? null : <CetakLaporan laporan={l} />}
          </div>
        ) : (
          <div className="hl__muat" aria-busy="true" aria-label="Memuat laporan">
            <span className="sk" style={{ width: '100%', height: 12 }} />
            <span className="sk" style={{ width: '50%', height: 32, marginTop: 40 }} />
            <span className="sk" style={{ width: '30%', height: 20 }} />
            <span className="sk" style={{ width: '100%', height: 88, marginTop: 32 }} />
            {[0, 1, 2, 3].map((i) => (
              <span key={i} className="sk" style={{ width: '100%', height: 16, marginTop: 8 }} />
            ))}
          </div>
        )}
      </main>
    </div>
  )
}
