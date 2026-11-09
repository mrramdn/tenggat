// Sign-in and sign-up pages (/masuk and /daftar): one form for both modes, with a slice of the product beside it on
// wide screens. The wordmark goes back to the front page.
import { useMutation } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { ApiError, auth, type Pengguna } from '../lib/api'
import { pergi, Tautan } from '../lib/rute'
import { Icon } from './Icon'
import { MasukPratinjau } from './MasukPratinjau'
import { pesanGalat } from './Toasts'

export type Mode = 'masuk' | 'daftar'
type Isian = 'nama' | 'email' | 'kata_sandi'

const ISIAN: Record<Mode, Isian[]> = {
  masuk: ['email', 'kata_sandi'],
  daftar: ['nama', 'email', 'kata_sandi'],
}

interface Props {
  /** masuk on /masuk, daftar on /daftar; switching changes the address and keeps ?lanjut. */
  mode: Mode
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

export function LayarMasuk({ mode, pemberitahuan, onMasuk }: Props) {
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`
  const [nama, setNama] = useState('')
  const [email, setEmail] = useState('')
  const [sandi, setSandi] = useState('')
  const [galat, setGalat] = useState<Partial<Record<Isian, string>>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)
  const tunggu = useTunggu()
  const formRef = useRef<HTMLFormElement>(null)

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

  // Focus the first field when the page appears and after switching mode; a switch keeps the email but not the
  // password or the errors of the other form.
  const modeAwal = useRef(mode)
  useEffect(() => {
    if (modeAwal.current !== mode) {
      modeAwal.current = mode
      setGalat({})
      setGalatUmum(null)
      setSandi('')
      kirim.reset()
    }
    formRef.current?.querySelector<HTMLInputElement>('input')?.focus()
  }, [mode])

  // Switching forms replaces the address, so Back does not bounce between /masuk and /daftar; ?lanjut stays.
  function gantiMode(m: Mode) {
    if (m !== mode) pergi(`/${m}${window.location.search}`, { ganti: true })
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
      <div className="auth__side">
        <header className="auth__top">
          <Tautan className="wordmark auth__merek" ke="/" aria-label="Tenggat, beranda">
            tenggat
          </Tautan>
        </header>
        <main className="auth__main">
          <h1 className="auth__title" id={id('judul')}>
            {daftar ? 'Buat akun Tenggat' : 'Masuk ke Tenggat'}
          </h1>

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

          <p className="auth__switch">
            {daftar ? 'Sudah punya akun?' : 'Belum punya akun?'}{' '}
            <a
              className="auth__link"
              href={`/${daftar ? 'masuk' : 'daftar'}${window.location.search}`}
              onClick={(e) => {
                if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return
                e.preventDefault()
                gantiMode(daftar ? 'masuk' : 'daftar')
              }}
              aria-disabled={sibuk || undefined}
            >
              {daftar ? 'Masuk' : 'Buat akun'}
            </a>
          </p>
        </main>
        <footer className="auth__foot">
          Tenggat menyimpan nama aset dan tanggal berakhirnya, tidak pernah nilai token atau kunci API.
        </footer>
      </div>
      <div className="auth__view">
        <MasukPratinjau />
      </div>
    </div>
  )
}
