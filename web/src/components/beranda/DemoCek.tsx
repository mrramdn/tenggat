// The front page's hero: one field, and the two dates Tenggat would keep for that site, read live from the
// registry and from the server. Nothing is saved.
import { useMutation } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { ApiError } from '../../lib/api'
import { publik, rapikanHost, type HasilCek } from '../../lib/api-publik'
import { frasaHari, tanggal } from '../../lib/format'
import { Icon } from '../Icon'
import { pesanGalat } from '../Toasts'
import { Pill } from '../ui'

const CONTOH = ['github.com', 'kompas.com', 'wikipedia.org']

/** Seconds left of a 429 wait, ticking down; 0 when the visitor may try again. */
function useTunggu() {
  const [sampai, setSampai] = useState<number | null>(null)
  const [kini, setKini] = useState(() => Date.now())
  useEffect(() => {
    if (sampai === null) return
    const t = window.setInterval(() => {
      const n = Date.now()
      setKini(n)
      if (n >= sampai) setSampai(null)
    }, 250)
    return () => window.clearInterval(t)
  }, [sampai])
  const mulai = (detik: number) => {
    const n = Date.now()
    setKini(n)
    setSampai(n + detik * 1000)
  }
  return { sisa: sampai === null ? 0 : Math.max(0, Math.ceil((sampai - kini) / 1000)), mulai }
}

/** API messages arrive as clauses ("registri domain ini belum menyediakan RDAP"); show them as sentences. */
function kalimat(s: string): string {
  const t = s.trim()
  if (!t) return t
  const awal = t.charAt(0).toUpperCase() + t.slice(1)
  return /[.!?]$/.test(awal) ? awal : `${awal}.`
}

export function DemoCek({ sudahMasuk }: { sudahMasuk: boolean }) {
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`
  const [isian, setIsian] = useState('')
  const [galatIsian, setGalatIsian] = useState<string | null>(null)
  const [galatUmum, setGalatUmum] = useState<string | null>(null)
  const tunggu = useTunggu()
  const inputRef = useRef<HTMLInputElement>(null)

  const cek = useMutation({
    mutationFn: (host: string) => publik.cek(host),
    onMutate: () => {
      setGalatIsian(null)
      setGalatUmum(null)
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 429) {
        if (err.retryAfter) tunggu.mulai(err.retryAfter)
        else setGalatUmum(pesanGalat(err))
        return
      }
      if (err instanceof ApiError && err.status === 422) {
        setGalatIsian(kalimat(err.field.host ?? err.message))
        inputRef.current?.focus()
        return
      }
      setGalatUmum(pesanGalat(err))
    },
  })

  function jalankan(mentah: string) {
    const host = rapikanHost(mentah)
    if (!host) {
      setGalatIsian('Ketik alamat situs, misalnya tokokuemawar.id.')
      inputRef.current?.focus()
      return
    }
    if (tunggu.sisa > 0 || cek.isPending) return
    if (host !== mentah) setIsian(host)
    cek.mutate(host)
  }

  const sibuk = cek.isPending
  const hasil = sibuk ? null : (cek.data ?? null)
  const dicek = sibuk ? cek.variables : hasil?.host

  return (
    <div className="demo">
      <form
        className="demo__form"
        role="search"
        aria-label="Cek tanggal domain dan sertifikat"
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          jalankan(isian)
        }}
      >
        <label className="sr-only" htmlFor={id('host')}>
          Alamat situs
        </label>
        <div className="demo__row">
          <div className="input-icon demo__field">
            <Icon name="globe" />
            <input
              ref={inputRef}
              id={id('host')}
              className="control is-lg"
              type="text"
              name="host"
              inputMode="url"
              autoComplete="url"
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              placeholder="Alamat situs, misalnya tokokuemawar.id"
              value={isian}
              onChange={(e) => {
                setIsian(e.target.value)
                if (galatIsian) setGalatIsian(null)
              }}
              aria-invalid={!!galatIsian}
              aria-describedby={galatIsian ? id('host-e') : undefined}
            />
          </div>
          <button type="submit" className="btn btn--primary is-lg demo__submit" disabled={tunggu.sisa > 0} aria-busy={sibuk}>
            {sibuk ? <Icon name="loader" className="spin" /> : null}
            {sibuk ? 'Mengecek' : 'Cek tanggal'}
          </button>
        </div>
        {galatIsian ? (
          <div className="field__error" id={id('host-e')}>
            <Icon name="alert" />
            <span>{galatIsian}</span>
          </div>
        ) : null}
        {tunggu.sisa > 0 ? (
          <div className="form-error" role="alert">
            <Icon name="alert" />
            <span>
              Terlalu banyak pengecekan dari jaringan Anda. Coba lagi dalam <span className="num">{tunggu.sisa}</span> detik.
            </span>
          </div>
        ) : galatUmum ? (
          <div className="form-error" role="alert">
            <Icon name="alert" />
            <span>{galatUmum}</span>
          </div>
        ) : null}
        <p className="demo__contoh">
          <span>Coba</span>
          {CONTOH.map((h) => (
            <button
              key={h}
              type="button"
              className="btn-teks"
              disabled={sibuk || tunggu.sisa > 0}
              onClick={() => {
                setIsian(h)
                setGalatIsian(null)
                jalankan(h)
              }}
            >
              {h}
            </button>
          ))}
        </p>
      </form>

      <section className="jawab" aria-labelledby={id('jawab')} aria-busy={sibuk}>
        <header className="jawab__top">
          <h2 className="jawab__judul" id={id('jawab')}>
            {dicek ? (
              <>
                <span className="jawab__muted">Hasil untuk</span> <b>{dicek}</b>
              </>
            ) : (
              <span className="jawab__muted">Belum ada alamat yang dicek</span>
            )}
          </h2>
          {hasil ? <span className="jawab__meta">Dibaca baru saja, tidak disimpan</span> : null}
        </header>
        <div className="jawab__isi" aria-live="polite">
          <Separuh
            jenis="domain"
            sibuk={sibuk}
            host={dicek}
            hasil={hasil}
          />
          <Separuh
            jenis="sertifikat"
            sibuk={sibuk}
            host={dicek}
            hasil={hasil}
          />
        </div>
        <footer className="jawab__kaki">
          {sudahMasuk ? (
            <p>
              Simpan keduanya sebagai aset di Tenggat agar dicek ulang berkala dan masuk ke kalender Anda.{' '}
              <a className="btn-teks" href="/app">
                Buka aplikasi
              </a>
            </p>
          ) : (
            <p>
              Simpan keduanya di Tenggat agar dicek ulang berkala dan masuk ke kalender Anda.{' '}
              <a className="btn-teks" href="/daftar">
                Buat akun
              </a>
            </p>
          )}
        </footer>
      </section>
    </div>
  )
}

interface SeparuhProps {
  jenis: 'domain' | 'sertifikat'
  sibuk: boolean
  host: string | undefined
  hasil: HasilCek | null
}

function Separuh({ jenis, sibuk, host, hasil }: SeparuhProps) {
  const domain = jenis === 'domain'
  const kepala = (
    <div className="jawab__label">
      <Icon name={domain ? 'globe' : 'lock'} />
      <span className="jawab__nama">{domain ? 'Domain' : 'Sertifikat TLS'}</span>
      <span className="jawab__sumber">{domain ? 'dari registri (RDAP)' : 'dari server, port 443'}</span>
    </div>
  )

  if (sibuk) {
    return (
      <div className="separuh is-loading">
        {kepala}
        <div className="separuh__status">
          <Icon name="loader" className="spin" />
          <span>{domain ? `Menanyakan ${host} ke registri` : `Menghubungi ${host}`}</span>
        </div>
        <span className="sk separuh__sk-big" />
        <span className="sk separuh__sk-line" />
      </div>
    )
  }

  if (!hasil) {
    return (
      <div className="separuh is-kosong">
        {kepala}
        <p className="separuh__jelas">
          {domain
            ? 'Tanggal pendaftaran domain berakhir. Lewat dari tanggal ini, situs berhenti bisa dibuka dan alamatnya bisa diambil orang lain.'
            : 'Tanggal sertifikat HTTPS berakhir. Lewat dari tanggal ini, browser menampilkan peringatan tidak aman kepada pengunjung.'}
        </p>
      </div>
    )
  }

  const d = domain ? hasil.domain : hasil.sertifikat
  const galat = domain ? hasil.domain_galat : hasil.sertifikat_galat
  if (!d) {
    return (
      <div className="separuh is-gagal">
        {kepala}
        <div className="separuh__gagal">
          <b>{domain ? 'Tanggal domain tidak terbaca.' : 'Sertifikat tidak terbaca.'}</b>
          <span>{galat ? kalimat(galat) : 'Coba lagi beberapa saat lagi.'}</span>
        </div>
        <p className="separuh__jelas">
          {domain
            ? 'Di Tenggat, tanggal seperti ini bisa Anda isi sendiri dari tagihan registrar.'
            : 'Pastikan situs bisa dibuka lewat HTTPS di port 443.'}
        </p>
      </div>
    )
  }

  return (
    <div className="separuh">
      {kepala}
      <div className="separuh__tgl">
        <span className="separuh__big num">{tanggal(d.kedaluwarsa)}</span>
        <span className="separuh__sisa">
          <span className={d.sisa_hari < 0 ? 'separuh__hari num is-lewat' : 'separuh__hari num'}>{frasaHari(d.sisa_hari)}</span>
          <Pill status={d.status} />
        </span>
      </div>
      <dl className="separuh__dl">
        {hasil.domain && domain ? (
          <>
            <dt>Domain</dt>
            <dd>{hasil.domain.domain}</dd>
            <dt>Registrar</dt>
            <dd>{hasil.domain.registrar || 'Tidak dicantumkan'}</dd>
          </>
        ) : hasil.sertifikat ? (
          <>
            <dt>Host</dt>
            <dd>{hasil.host}</dd>
            <dt>Penerbit</dt>
            <dd>{hasil.sertifikat.penerbit || 'Tidak dicantumkan'}</dd>
          </>
        ) : null}
      </dl>
    </div>
  )
}
