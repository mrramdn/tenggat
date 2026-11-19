// Personal access tokens in Pengaturan: list active tokens, create a token (shown once in component state with a
// ready-to-copy MCP command), and revoke tokens with confirmation.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type FormEvent, type RefObject } from 'react'
import { ApiError } from '../lib/api'
import {
  KUNCI_TOKEN,
  tokenApi,
  type CakupanToken,
  type TokenAkses,
  type TokenBaru,
  type TokenInput,
} from '../lib/api-token'
import { lalu, tanggal } from '../lib/format'
import { ConfirmDialog } from './ConfirmDialog'
import { Icon } from './Icon'
import { Pilihan, type OpsiPilihan } from './Pilihan'
import { pesanGalat, useToast } from './Toasts'
import '../styles/token.css'

type PilihanBerlaku = '30' | '90' | '365' | 'tanpa'

const opsiBerlaku: OpsiPilihan<PilihanBerlaku>[] = [
  { nilai: '30', label: '30 hari' },
  { nilai: '90', label: '90 hari' },
  { nilai: '365', label: '365 hari' },
  { nilai: 'tanpa', label: 'Tanpa batas' },
]

// Nothing is selected at first: a read-only default silently hides the tools that
// add data, and a write default hands out more than was asked for, so the user picks.
const opsiCakupan: { v: CakupanToken; label: string }[] = [
  { v: 'tulis', label: 'Baca dan tulis' },
  { v: 'baca', label: 'Baca saja' },
]

const MAKS_TOKEN = 20

function tanggalWaktu(ts: string): string {
  const d = new Date(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return tanggal(`${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`)
}

function labelTerakhirDipakai(ts: string | null): string {
  if (!ts) return 'Terakhir dipakai: belum pernah'
  const umur = Date.now() - new Date(ts).getTime()
  if (umur >= 0 && umur < 30 * 86_400_000) return `Terakhir dipakai ${lalu(ts)}`
  return `Terakhir dipakai ${tanggalWaktu(ts)}`
}

export function TokenPengaturan() {
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`
  const qc = useQueryClient()
  const toast = useToast()

  const q = useQuery({ queryKey: KUNCI_TOKEN, queryFn: tokenApi.daftar })

  // Kept only in component state (never in query cache or storage) so the secret is gone when leaving this page.
  const [baru, setBaru] = useState<TokenBaru | null>(null)
  const [bukaForm, setBukaForm] = useState(false)
  const [cabutTarget, setCabutTarget] = useState<TokenAkses | null>(null)

  const [nama, setNama] = useState('')
  const [cakupan, setCakupan] = useState<CakupanToken | null>(null)
  const [berlaku, setBerlaku] = useState<PilihanBerlaku>('90')
  const [galat, setGalat] = useState<Record<string, string>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)

  const [tersalinToken, setTersalinToken] = useState(false)
  const [tersalinPerintah, setTersalinPerintah] = useState(false)
  const namaRef = useRef<HTMLInputElement>(null)
  const tokenRef = useRef<HTMLInputElement>(null)
  const perintahRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (!tersalinToken) return
    const t = window.setTimeout(() => setTersalinToken(false), 2000)
    return () => window.clearTimeout(t)
  }, [tersalinToken])

  useEffect(() => {
    if (!tersalinPerintah) return
    const t = window.setTimeout(() => setTersalinPerintah(false), 2000)
    return () => window.clearTimeout(t)
  }, [tersalinPerintah])

  const buat = useMutation({
    mutationFn: (input: TokenInput) => tokenApi.buat(input),
    onSuccess: (res) => {
      setBaru(res)
      setBukaForm(false)
      setNama('')
      setCakupan(null)
      setBerlaku('90')
      setGalat({})
      setGalatUmum(null)
      setTersalinToken(false)
      setTersalinPerintah(false)
      void qc.invalidateQueries({ queryKey: KUNCI_TOKEN })
      toast.ok('Token dibuat.', 'Salin token sekarang; tidak ditampilkan lagi.')
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 409) {
        void qc.invalidateQueries({ queryKey: KUNCI_TOKEN })
      }
      if (err instanceof ApiError && Object.keys(err.field).length > 0) {
        setGalat(err.field)
        setGalatUmum(null)
      } else {
        setGalatUmum(pesanGalat(err))
      }
    },
  })

  const cabut = useMutation({
    mutationFn: (t: TokenAkses) => tokenApi.cabut(t.id),
    onSuccess: (_, t) => {
      setCabutTarget(null)
      if (baru?.token.id === t.id) setBaru(null)
      void qc.invalidateQueries({ queryKey: KUNCI_TOKEN })
      toast.ok('Token dicabut.', t.nama)
    },
    onError: (err) => toast.dariGalat('Token tidak dapat dicabut.', err),
  })

  async function salin(
    teks: string,
    ref: RefObject<HTMLInputElement | HTMLTextAreaElement | null>,
    setStatus: (v: boolean) => void,
    label: string,
  ) {
    try {
      await navigator.clipboard.writeText(teks)
      setStatus(true)
    } catch {
      ref.current?.focus()
      ref.current?.select()
      toast.galat(`${label} tidak dapat disalin otomatis.`, `${label} sudah dipilih. Salin dengan Ctrl+C atau menu salin.`)
    }
  }

  function kirim(e: FormEvent) {
    e.preventDefault()
    if (buat.isPending) return
    const bersih = nama.trim()
    if (!bersih) {
      setGalat({ nama: 'Isi nama token.' })
      namaRef.current?.focus()
      return
    }
    if (!cakupan) {
      setGalat({ cakupan: 'Pilih cakupan token ini.' })
      return
    }
    setGalat({})
    setGalatUmum(null)
    const input: TokenInput = { nama: bersih, cakupan }
    // Omit berlaku_hari for the default 90 days; send null for no expiry, or 30/365.
    if (berlaku === 'tanpa') input.berlaku_hari = null
    else if (berlaku !== '90') input.berlaku_hari = Number(berlaku)
    buat.mutate(input)
  }

  function tutupForm() {
    setBukaForm(false)
    setNama('')
    setCakupan(null)
    setBerlaku('90')
    setGalat({})
    setGalatUmum(null)
  }

  const list = q.data ?? []
  const penuh = list.length >= MAKS_TOKEN
  const perintah = baru
    ? `claude mcp add --transport http tenggat ${window.location.origin}/mcp --header "Authorization: Bearer ${baru.rahasia}"`
    : ''

  let daftar
  if (q.isPending) {
    daftar = (
      <div className="tok" aria-busy="true" aria-label="Memuat token akses">
        <span className="sk" style={{ width: '100%', height: 56 }} />
      </div>
    )
  } else if (q.isError) {
    daftar = (
      <div className="form-error" role="alert">
        <Icon name="alert" />
        <span>
          Daftar token tidak dapat dimuat. {pesanGalat(q.error)}{' '}
          <button type="button" className="btn-teks is-danger" onClick={() => void q.refetch()}>
            Coba lagi
          </button>
        </span>
      </div>
    )
  } else if (list.length === 0) {
    daftar = <p className="tok__kosong">Belum ada token akses.</p>
  } else {
    const sekarang = Date.now()
    daftar = (
      <ul className="tok__list" aria-label="Daftar token akses">
        {list.map((t) => {
          const lewat = t.kedaluwarsa !== null && new Date(t.kedaluwarsa).getTime() <= sekarang
          const labelKedaluwarsa =
            t.kedaluwarsa === null ? 'Tanpa batas' : lewat ? 'Kedaluwarsa' : `Berlaku sampai ${tanggalWaktu(t.kedaluwarsa)}`
          return (
            <li key={t.id} className="tok__item">
              <div className="tok__main">
                <div className="tok__judul">
                  <span className="tok__nama">{t.nama}</span>
                  <code className="tok__prefix">{t.awalan}…</code>
                  <span className={t.cakupan === 'tulis' ? 'tok__cakupan is-tulis' : 'tok__cakupan'}>
                    {t.cakupan === 'tulis' ? 'Baca dan tulis' : 'Baca saja'}
                  </span>
                </div>
                <div className="tok__meta num">
                  <span>Dibuat {tanggalWaktu(t.dibuat)}</span>
                  <span>{labelTerakhirDipakai(t.terakhir_dipakai)}</span>
                  <span
                    className={lewat ? 'tok__exp is-lewat' : 'tok__exp'}
                    title={lewat && t.kedaluwarsa ? `Berakhir ${tanggalWaktu(t.kedaluwarsa)}` : undefined}
                  >
                    {labelKedaluwarsa}
                  </span>
                </div>
              </div>
              <button
                type="button"
                className="btn btn--secondary is-sm is-danger tok__cabut"
                onClick={() => setCabutTarget(t)}
                aria-label={`Cabut token ${t.nama}`}
              >
                Cabut
              </button>
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <div className="tok">
      {baru ? (
        <div className="tok__baru" role="region" aria-live="polite" aria-label="Token baru">
          <div className="tok__baru-head">
            <div className="tok__baru-judul">
              <b>{baru.token.nama}</b>
              <span className="tok__sekali">
                <Icon name="alert" />
                <span>Token ini hanya ditampilkan sekali.</span>
              </span>
            </div>
            <button
              type="button"
              className="btn btn--ghost is-sm btn--icon"
              aria-label="Tutup tampilan token"
              onClick={() => setBaru(null)}
            >
              <Icon name="x" />
            </button>
          </div>
          <div className="field">
            <label className="field__label" htmlFor={id('rahasia')}>
              Token
            </label>
            <div className="field-row">
              <input
                ref={tokenRef}
                id={id('rahasia')}
                className="control tok__mono"
                readOnly
                value={baru.rahasia}
                spellCheck={false}
                onFocus={(e) => e.currentTarget.select()}
              />
              <button
                type="button"
                className="btn btn--secondary tok__salin"
                onClick={() => void salin(baru.rahasia, tokenRef, setTersalinToken, 'Token')}
              >
                <Icon name={tersalinToken ? 'check' : 'copy'} />
                {tersalinToken ? 'Tersalin' : 'Salin'}
              </button>
            </div>
          </div>
          <div className="field">
            <label className="field__label" htmlFor={id('perintah')}>
              Perintah Claude Code
            </label>
            <div className="tok__cmd-box">
              <textarea
                ref={perintahRef}
                id={id('perintah')}
                className="control tok__cmd"
                readOnly
                rows={2}
                value={perintah}
                spellCheck={false}
                onFocus={(e) => e.currentTarget.select()}
              />
              <button
                type="button"
                className="btn btn--secondary tok__salin"
                onClick={() => void salin(perintah, perintahRef, setTersalinPerintah, 'Perintah')}
              >
                <Icon name={tersalinPerintah ? 'check' : 'copy'} />
                {tersalinPerintah ? 'Tersalin' : 'Salin'}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {bukaForm ? (
        <form className="tok__form" onSubmit={kirim} noValidate aria-label="Buat token akses">
          {galatUmum ? (
            <div className="form-error" role="alert">
              <Icon name="alert" />
              <span>{galatUmum}</span>
            </div>
          ) : null}
          <div className="field">
            <label className="field__label" htmlFor={id('nama')}>
              Nama token
            </label>
            <input
              ref={namaRef}
              id={id('nama')}
              className="control"
              placeholder="Contoh: Claude Code laptop"
              autoComplete="off"
              maxLength={60}
              value={nama}
              autoFocus
              aria-invalid={!!galat.nama}
              aria-describedby={galat.nama ? id('nama-e') : undefined}
              onChange={(e) => {
                setNama(e.target.value)
                if (galat.nama) setGalat((g) => ({ ...g, nama: '' }))
              }}
            />
            {galat.nama ? (
              <div className="field__error" id={id('nama-e')}>
                <Icon name="alert" />
                <span>{galat.nama}</span>
              </div>
            ) : null}
          </div>
          <div className="field">
            <span className="field__label" id={id('cakupan-l')}>
              Cakupan
            </span>
            <div
              className={galat.cakupan ? 'seg is-full is-error' : 'seg is-full'}
              role="radiogroup"
              aria-labelledby={id('cakupan-l')}
            >
              {opsiCakupan.map((o) => (
                <button
                  key={o.v}
                  type="button"
                  className="seg__item"
                  role="radio"
                  aria-checked={cakupan === o.v}
                  tabIndex={cakupan === o.v || (cakupan === null && o.v === opsiCakupan[0].v) ? 0 : -1}
                  data-cakupan={o.v}
                  onClick={() => {
                    setCakupan(o.v)
                    if (galat.cakupan) setGalat((g) => ({ ...g, cakupan: '' }))
                  }}
                  onKeyDown={(e) => {
                    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
                    e.preventDefault()
                    const lain: CakupanToken = o.v === 'baca' ? 'tulis' : 'baca'
                    setCakupan(lain)
                    if (galat.cakupan) setGalat((g) => ({ ...g, cakupan: '' }))
                    const sib = e.currentTarget.parentElement?.querySelector<HTMLElement>(`[data-cakupan="${lain}"]`)
                    sib?.focus()
                  }}
                >
                  {o.label}
                </button>
              ))}
            </div>
            {galat.cakupan ? (
              <div className="field__error" id={id('cakupan-e')}>
                <Icon name="alert" />
                <span>{galat.cakupan}</span>
              </div>
            ) : (
              <div className="field__hint">
                {cakupan === null
                  ? 'Baca dan tulis untuk alat yang Anda minta menambah atau menandai data; Baca saja untuk yang hanya melihat.'
                  : cakupan === 'baca'
                    ? 'Hanya melihat agenda, aset, klien, tugas, dan aktivitas. Alat ini tidak bisa menambah atau mengubah data.'
                    : 'Melihat serta menambah, mengubah, dan menandai selesai data Anda.'}
              </div>
            )}
          </div>
          <div className="field">
            <label className="field__label" htmlFor={id('berlaku')}>
              Masa berlaku
            </label>
            <Pilihan
              id={id('berlaku')}
              nilai={berlaku}
              opsi={opsiBerlaku}
              invalid={!!galat.berlaku_hari}
              describedBy={galat.berlaku_hari ? id('berlaku-e') : berlaku === 'tanpa' ? id('berlaku-w') : undefined}
              onPilih={(v) => {
                setBerlaku(v)
                if (galat.berlaku_hari) setGalat((g) => ({ ...g, berlaku_hari: '' }))
              }}
            />
            {galat.berlaku_hari ? (
              <div className="field__error" id={id('berlaku-e')}>
                <Icon name="alert" />
                <span>{galat.berlaku_hari}</span>
              </div>
            ) : berlaku === 'tanpa' ? (
              <div className="tok__warn" id={id('berlaku-w')} role="note">
                <Icon name="alert" />
                <span>Token tanpa batas berlaku selamanya sampai Anda mencabutnya.</span>
              </div>
            ) : null}
          </div>
          <div className="tok__form-aksi">
            <button type="button" className="btn btn--secondary" onClick={tutupForm} disabled={buat.isPending}>
              Batal
            </button>
            <button type="submit" className="btn btn--primary" disabled={buat.isPending} aria-busy={buat.isPending}>
              {buat.isPending ? <Icon name="loader" className="spin" /> : null}
              {buat.isPending ? 'Membuat' : 'Buat token'}
            </button>
          </div>
        </form>
      ) : (
        <div className="tok__bar">
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => setBukaForm(true)}
            disabled={q.isPending || penuh}
          >
            <Icon name="plus" />
            Buat token
          </button>
          {q.data ? (
            <span className="tok__hitung num">
              {penuh ? `Batas ${MAKS_TOKEN} token tercapai` : `${list.length} dari ${MAKS_TOKEN} token`}
            </span>
          ) : null}
        </div>
      )}

      {daftar}

      {cabutTarget ? (
        <ConfirmDialog
          judul="Cabut token?"
          tombol="Cabut token"
          tombolSedang="Mencabut"
          sedang={cabut.isPending}
          onBatal={() => setCabutTarget(null)}
          onYa={() => cabut.mutate(cabutTarget)}
        >
          <b>{cabutTarget.nama}</b> (<code className="tok__code">{cabutTarget.awalan}…</code>) langsung berhenti bekerja.
          Alat yang memakai token ini tidak dapat lagi mengakses Tenggat.
        </ConfirmDialog>
      ) : null}
    </div>
  )
}
