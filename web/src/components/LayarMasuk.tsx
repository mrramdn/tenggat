// Sign-in screen: "Masuk" and "Daftar" in one card, with a short pitch beside it
// (desktop) or below it (phone). Shown whenever there is no valid session.
import { useMutation } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { ApiError, auth, type Pengguna } from '../lib/api'
import { Icon } from './Icon'
import { pesanGalat } from './Toasts'

type Mode = 'masuk' | 'daftar'
type Isian = 'nama' | 'email' | 'kata_sandi'

const ISIAN: Record<Mode, Isian[]> = {
  masuk: ['email', 'kata_sandi'],
  daftar: ['nama', 'email', 'kata_sandi'],
}

interface Props {
  /** A note shown above the form, e.g. after the session expired. */
  pemberitahuan: string | null
  onMasuk: (p: Pengguna, baru: boolean) => void
}

function FieldError({ id, pesan }: { id: string; pesan?: string }) {
  if (!pesan) return null
  return (
    <div className="field__error" id={id}>
      <Icon name="alert" />
      <span>{pesan}</span>
    </div>
  )
}

/** Seconds left of a 429 wait, ticking down; 0 when the user may try again. */
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

const POIN: { icon: 'server' | 'calendar' | 'shield'; judul: string; teks: string }[] = [
  { icon: 'server', judul: 'Sertifikat dibaca dari server', teks: 'Ketik host-nya saja. Tanggal berakhirnya terbaca langsung dan dicek ulang setiap 6 jam.' },
  { icon: 'calendar', judul: 'Pengingat di kalender ponsel', teks: 'Unduh kalender (.ics), tenggat muncul di aplikasi kalender Anda.' },
  { icon: 'shield', judul: 'Hanya nama dan tanggal yang disimpan', teks: 'Tanpa nilai token, kunci API, atau data pembayaran.' },
]

export function LayarMasuk({ pemberitahuan, onMasuk }: Props) {
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`
  const [mode, setMode] = useState<Mode>('masuk')
  const [nama, setNama] = useState('')
  const [email, setEmail] = useState('')
  const [sandi, setSandi] = useState('')
  const [galat, setGalat] = useState<Partial<Record<Isian, string>>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)
  const tunggu = useTunggu()
  const formRef = useRef<HTMLFormElement>(null)

  // Focus the first field when the screen appears and after switching mode.
  useEffect(() => {
    formRef.current?.querySelector<HTMLInputElement>('input')?.focus()
  }, [mode])

  const fokus = (f: Isian) => window.requestAnimationFrame(() => document.getElementById(id(f))?.focus())

  const kirim = useMutation({
    mutationFn: (m: Mode) =>
      m === 'daftar'
        ? auth.daftar({ nama: nama.trim(), email: email.trim(), kata_sandi: sandi })
        : auth.masuk({ email: email.trim(), kata_sandi: sandi }),
    onSuccess: (p, m) => onMasuk(p, m === 'daftar'),
    onError: (err, m) => {
      if (err instanceof ApiError && err.status === 429 && err.retryAfter) {
        tunggu.mulai(err.retryAfter)
        setGalatUmum(null)
        return
      }
      if (err instanceof ApiError && Object.keys(err.field).length > 0) {
        const tampil = ISIAN[m]
        const ada = tampil.filter((f) => err.field[f])
        const lain = Object.entries(err.field).filter(([k]) => !tampil.includes(k as Isian))
        setGalat(Object.fromEntries(ada.map((f) => [f, err.field[f]])))
        setGalatUmum(lain.length ? lain.map(([, v]) => v).join(' ') : ada.length ? null : err.message)
        if (ada[0]) fokus(ada[0])
        return
      }
      setGalat({})
      setGalatUmum(pesanGalat(err))
      if (err instanceof ApiError && err.status === 401) {
        setSandi('')
        fokus('kata_sandi')
      }
    },
  })

  function gantiMode(m: Mode) {
    if (m === mode) return
    setMode(m)
    setGalat({})
    setGalatUmum(null)
    setSandi('')
    kirim.reset()
  }

  function ubah(f: Isian, v: string) {
    if (f === 'nama') setNama(v)
    else if (f === 'email') setEmail(v)
    else setSandi(v)
    if (galat[f]) setGalat((g) => ({ ...g, [f]: undefined }))
  }

  const sibuk = kirim.isPending
  const bisaKirim = !sibuk && tunggu.sisa === 0
  const daftar = mode === 'daftar'

  const field = (f: Isian, label: string, input: { type: string; autoComplete: string; extra?: object }, hint?: ReactNode) => (
    <div className="field">
      <label className="field__label" htmlFor={id(f)}>
        {label}
      </label>
      <input
        id={id(f)}
        className="control"
        type={input.type}
        name={f === 'kata_sandi' ? 'password' : f}
        autoComplete={input.autoComplete}
        value={f === 'nama' ? nama : f === 'email' ? email : sandi}
        onChange={(e) => ubah(f, e.target.value)}
        aria-invalid={!!galat[f]}
        aria-describedby={galat[f] ? id(`${f}-e`) : hint ? id(`${f}-h`) : undefined}
        {...input.extra}
      />
      {galat[f] ? (
        <FieldError id={id(`${f}-e`)} pesan={galat[f]} />
      ) : hint ? (
        <div className="field__hint" id={id(`${f}-h`)}>
          {hint}
        </div>
      ) : null}
    </div>
  )

  return (
    <div className="auth">
      <header className="auth__top">
        <div className="wordmark">tenggat</div>
      </header>
      <main className="auth__main">
        <section className="auth__pitch" aria-labelledby={id('pitch')}>
          <h1 className="display" id={id('pitch')}>
            Semua tenggat Anda. <span className="quiet">Satu daftar.</span>
          </h1>
          <p className="auth__lede">
            Domain, sertifikat TLS, token API, dan langganan semua klien Anda, diurutkan menurut sisa hari.
          </p>
          <ul className="auth__points">
            {POIN.map((p) => (
              <li key={p.judul}>
                <Icon name={p.icon} />
                <div>
                  <b>{p.judul}</b>
                  <span>{p.teks}</span>
                </div>
              </li>
            ))}
          </ul>
        </section>

        <section className="auth__panel" aria-labelledby={id('judul')}>
          <div className="auth__card">
            <div className="seg is-full" role="radiogroup" aria-label="Pilih masuk atau daftar">
              {(['masuk', 'daftar'] as const).map((m) => (
                <button
                  key={m}
                  type="button"
                  className="seg__item"
                  role="radio"
                  aria-checked={mode === m}
                  disabled={sibuk}
                  onClick={() => gantiMode(m)}
                >
                  {m === 'masuk' ? 'Masuk' : 'Daftar'}
                </button>
              ))}
            </div>
            <div className="auth__head">
              <h2 className="auth__title" id={id('judul')}>
                {daftar ? 'Buat akun Tenggat' : 'Masuk ke Tenggat'}
              </h2>
              <p className="auth__sub">
                {daftar ? 'Cukup nama, email, dan kata sandi.' : 'Lanjutkan memantau tenggat Anda.'}
              </p>
            </div>

            {pemberitahuan && !galatUmum && tunggu.sisa === 0 ? (
              <div className="notice" role="status">
                <Icon name="info" />
                <span>{pemberitahuan}</span>
              </div>
            ) : null}
            {tunggu.sisa > 0 ? (
              <div className="form-error" role="alert">
                <Icon name="alert" />
                <span>
                  Terlalu banyak percobaan. Coba lagi dalam <span className="num">{tunggu.sisa}</span> detik.
                </span>
              </div>
            ) : galatUmum ? (
              <div className="form-error" role="alert">
                <Icon name="alert" />
                <span>{galatUmum}</span>
              </div>
            ) : null}

            <form
              ref={formRef}
              className="auth__form"
              noValidate
              aria-labelledby={id('judul')}
              onSubmit={(e) => {
                e.preventDefault()
                if (!bisaKirim) return
                setGalatUmum(null)
                kirim.mutate(mode)
              }}
            >
              {daftar ? field('nama', 'Nama', { type: 'text', autoComplete: 'name', extra: { autoCapitalize: 'words' } }) : null}
              {field('email', 'Email', {
                type: 'email',
                autoComplete: 'email',
                extra: { inputMode: 'email', autoCapitalize: 'off', spellCheck: false },
              })}
              {field(
                'kata_sandi',
                'Kata sandi',
                { type: 'password', autoComplete: daftar ? 'new-password' : 'current-password' },
                daftar ? 'Minimal 8 karakter.' : undefined,
              )}
              <button type="submit" className="btn btn--primary auth__submit" disabled={!bisaKirim} aria-busy={sibuk}>
                {sibuk ? <Icon name="loader" className="spin" /> : null}
                {daftar ? (sibuk ? 'Membuat akun' : 'Buat akun') : sibuk ? 'Sedang masuk' : 'Masuk'}
              </button>
            </form>
          </div>
          <div className="privacy">
            <Icon name="shield" />
            <span>Tenggat hanya menyimpan nama dan tanggal. Jangan simpan nilai token atau kata sandi.</span>
          </div>
        </section>
      </main>
    </div>
  )
}
