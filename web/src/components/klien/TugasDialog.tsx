// Dialogs for tasks: "Tambah tugas" / "Tambah tenggat" / "Ubah tugas", and "Tandai selesai" (date, minutes, a note).
import { useMutation } from '@tanstack/react-query'
import { useId, useRef, useState, type ReactNode } from 'react'
import { ApiError } from '../../lib/api'
import {
  jatuhTempoBerikutnya,
  tambahInterval,
  tugasApi,
  useSegarkan,
  type MataUang,
  type SatuanUlang,
  type Tugas,
  type TugasInput,
} from '../../lib/api-tugas'
import { angka, hariIniIso, tanggal } from '../../lib/format'
import { useModal } from '../../hooks/useModal'
import { Icon } from '../Icon'
import { pesanGalat, useToast } from '../Toasts'
import { contohTugas, type ContohTugas } from './contoh-tugas'
import '../../styles/tugas.css'
import { Pilihan } from '../Pilihan'

const satuan: { v: SatuanUlang; label: string }[] = [
  { v: 'hari', label: 'hari' },
  { v: 'minggu', label: 'minggu' },
  { v: 'bulan', label: 'bulan' },
  { v: 'tahun', label: 'tahun' },
]

const pilihanJadwal: { v: boolean; label: string }[] = [
  { v: true, label: 'Sekali' },
  { v: false, label: 'Berulang' },
]

const pilihanUang: { v: MataUang; label: string; aria: string }[] = [
  { v: 'IDR', label: 'Rp', aria: 'Rupiah (IDR)' },
  { v: 'USD', label: 'US$', aria: 'Dolar AS (USD)' },
]

const MAKS_BIAYA = 1_000_000_000_000

function FieldError({ id, pesan }: { id: string; pesan?: string }) {
  if (!pesan) return null
  return (
    <div className="field__error" id={id}>
      <Icon name="alert" />
      <span>{pesan}</span>
    </div>
  )
}

function Kerangka({ judul, onClose, children, onSubmit, lebar = false, ketikLangsung = false }: { judul: string; onClose: () => void; children: ReactNode; onSubmit: () => void; lebar?: boolean; ketikLangsung?: boolean }) {
  const ref = useRef<HTMLDivElement>(null)
  useModal(ref, onClose)
  const id = useId()
  return (
    <>
      <div className="scrim" style={{ zIndex: 32 }} onClick={onClose} aria-hidden="true" />
      <div ref={ref} className={lebar ? 'dialog tg-dialog is-wide' : 'dialog tg-dialog'} style={{ zIndex: 33 }} data-ketik-langsung={ketikLangsung ? '' : undefined} role="dialog" aria-modal="true" aria-labelledby={id}>
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            onSubmit()
          }}
        >
          <h2 className="dialog__title" id={id}>
            {judul}
          </h2>
          {children}
        </form>
      </div>
    </>
  )
}

function GalatUmum({ pesan }: { pesan: string | null }) {
  if (!pesan) return null
  return (
    <div className="form-error" role="alert">
      <Icon name="alert" />
      <span>{pesan}</span>
    </div>
  )
}

function Kaki({ onClose, sedang, label, labelSedang, bisa }: { onClose: () => void; sedang: boolean; label: string; labelSedang: string; bisa: boolean }) {
  return (
    <div className="dialog__foot">
      <button type="button" className="btn btn--secondary" onClick={onClose}>
        Batal
      </button>
      <button type="submit" className="btn btn--primary" disabled={!bisa} aria-busy={sedang}>
        {sedang ? <Icon name="loader" className="spin" /> : null}
        {sedang ? labelSedang : label}
      </button>
    </div>
  )
}

/** Moves focus to the first field marked invalid once it renders. */
const fokusGalat = () =>
  requestAnimationFrame(() => document.querySelector<HTMLElement>('.tg-dialog [aria-invalid="true"]')?.focus())

const hanyaAngka = (s: string, maks: number) => s.replace(/\D/g, '').replace(/^0+(?=\d)/, '').slice(0, maks)

/** Keeps plain digits with an optional decimal point and at most 2 fractional digits (for USD). */
function hanyaDesimal(s: string): string {
  const bersih = s.replace(',', '.').replace(/[^0-9.]/g, '')
  const titik = bersih.indexOf('.')
  if (titik === -1) return bersih.replace(/^0+(?=\d)/, '').slice(0, 13)
  const bulat = bersih.slice(0, titik).replace(/^0+(?=\d)/, '').slice(0, 13)
  const pecah = bersih.slice(titik + 1).replace(/\./g, '').slice(0, 2)
  return `${bulat || '0'}.${pecah}`
}

interface TugasDialogProps {
  /** Optional client id; omit for a personal deadline without a client. */
  klienId?: string
  /** The task to edit; without it the dialog adds one. */
  tugas?: Tugas
  /** Prefill for a new task, from a suggestion. */
  contoh?: ContohTugas
  /** Preselect "Sekali" when adding a new task; defaults to false (Berulang). */
  awalSekali?: boolean
  onClose: () => void
  onSaved: (t: Tugas, baru: boolean) => void
}

export function TugasDialog({ klienId, tugas, contoh, awalSekali = false, onClose, onSaved }: TugasDialogProps) {
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`
  const segarkan = useSegarkan()
  const efektifKlienId = tugas ? (tugas.klien_id ?? undefined) : klienId
  const awalSatuan = tugas?.ulang_satuan ?? contoh?.ulang_satuan ?? 'bulan'
  const awalJumlah = tugas?.ulang_jumlah ?? contoh?.ulang_jumlah ?? 1
  const awalUang: MataUang = tugas?.mata_uang === 'USD' ? 'USD' : 'IDR'

  const [judul, setJudul] = useState(tugas?.judul ?? contoh?.judul ?? '')
  const [sekali, setSekali] = useState(tugas ? tugas.sekali : awalSekali)
  const [jumlah, setJumlah] = useState(String(awalJumlah))
  const [unit, setUnit] = useState<SatuanUlang>(awalSatuan)
  const [jatuhTempo, setJatuhTempo] = useState(
    tugas?.jatuh_tempo ?? (contoh ? tambahInterval(hariIniIso(), contoh.ulang_jumlah, contoh.ulang_satuan) : ''),
  )
  const [mataUang, setMataUang] = useState<MataUang>(awalUang)
  const [biaya, setBiaya] = useState(
    tugas?.biaya != null ? (awalUang === 'USD' ? String(tugas.biaya) : String(Math.round(tugas.biaya))) : '',
  )
  const [catatan, setCatatan] = useState(tugas?.catatan ?? '')
  const [galat, setGalat] = useState<Record<string, string>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)

  const simpan = useMutation({
    mutationFn: (input: TugasInput) => (tugas ? tugasApi.ubah(tugas.id, input) : tugasApi.buat(input)),
    onSuccess: (t) => onSaved(t, !tugas),
    onError: (err) => {
      if (err instanceof ApiError && err.status === 409) {
        segarkan()
        setGalatUmum(pesanGalat(err))
        return
      }
      if (err instanceof ApiError && Object.keys(err.field).length > 0) {
        const f = { ...err.field }
        if (f.ulang_satuan && !f.ulang_jumlah) f.ulang_jumlah = f.ulang_satuan
        if (f.mata_uang && !f.biaya) f.biaya = f.mata_uang
        setGalat(f)
        fokusGalat()
      } else setGalatUmum(pesanGalat(err))
    },
  })

  const hapusGalat = (k: string) => setGalat((g) => (g[k] ? { ...g, [k]: '' } : g))
  const n = Number(jumlah)

  function gantiMataUang(baru: MataUang) {
    if (baru === mataUang) return
    setMataUang(baru)
    hapusGalat('biaya')
    if (baru === 'IDR' && biaya !== '') {
      setBiaya(hanyaAngka(biaya.split('.')[0], 13))
    }
  }

  function kirim() {
    const g: Record<string, string> = {}
    if (judul.trim().length < 2) g.judul = 'Isi judul, minimal 2 huruf.'
    if (!sekali && !(n >= 1 && n <= 365)) g.ulang_jumlah = 'Isi angka 1 sampai 365.'
    if (!jatuhTempo) g.jatuh_tempo = 'Pilih tanggal jatuh tempo.'
    let nilaiBiaya: number | null = null
    if (biaya !== '') {
      const valid = mataUang === 'USD' ? /^\d+(\.\d{1,2})?$/.test(biaya) : /^\d+$/.test(biaya)
      const angkaBiaya = Number(biaya)
      if (!valid || !Number.isFinite(angkaBiaya) || angkaBiaya < 0 || angkaBiaya > MAKS_BIAYA) {
        g.biaya =
          mataUang === 'USD'
            ? 'Isi angka 0 atau lebih, paling banyak 2 desimal.'
            : 'Isi angka 0 atau lebih tanpa desimal.'
      } else {
        nilaiBiaya = angkaBiaya
      }
    }
    if (Object.keys(g).length > 0) {
      setGalat(g)
      fokusGalat()
      return
    }
    setGalatUmum(null)
    const input: TugasInput = {
      ...(efektifKlienId ? { klien_id: efektifKlienId } : {}),
      judul: judul.trim(),
      catatan: catatan.trim(),
      sekali,
      ...(sekali ? {} : { ulang_jumlah: n, ulang_satuan: unit }),
      jatuh_tempo: jatuhTempo,
      biaya: nilaiBiaya,
      mata_uang: mataUang,
    }
    simpan.mutate(input)
  }

  function pakaiContoh(c: ContohTugas) {
    setJudul(c.judul)
    setSekali(false)
    setJumlah(String(c.ulang_jumlah))
    setUnit(c.ulang_satuan)
    if (!jatuhTempo) setJatuhTempo(tambahInterval(hariIniIso(), c.ulang_jumlah, c.ulang_satuan))
    setGalat({})
  }

  const judulDialog = tugas
    ? efektifKlienId
      ? 'Ubah tugas'
      : 'Ubah tenggat'
    : efektifKlienId
      ? 'Tambah tugas'
      : 'Tambah tenggat'

  const ulangValid = !sekali && n >= 1 && n <= 365

  const kolomTanggal = (
    <div className="field">
      <label className="field__label" htmlFor={id('tgl')}>
        {sekali || tugas ? 'Jatuh tempo' : 'Jatuh tempo pertama'}
      </label>
      <label className="input-icon">
        <Icon name="calendar" />
        <input
          id={id('tgl')}
          type="date"
          className={jatuhTempo ? 'control num has-value' : 'control num'}
          value={jatuhTempo}
          min="2000-01-01"
          max="2100-12-31"
          aria-invalid={!!galat.jatuh_tempo}
          aria-describedby={galat.jatuh_tempo ? id('tgl-e') : undefined}
          onChange={(e) => {
            setJatuhTempo(e.target.value)
            hapusGalat('jatuh_tempo')
          }}
        />
      </label>
      <FieldError id={id('tgl-e')} pesan={galat.jatuh_tempo} />
    </div>
  )

  return (
    <Kerangka judul={judulDialog} onClose={onClose} onSubmit={kirim} lebar ketikLangsung={!tugas}>
      <GalatUmum pesan={galatUmum} />
      <div className="field">
        <label className="field__label" htmlFor={id('judul')}>
          Judul
        </label>
        <input
          id={id('judul')}
          className="control"
          placeholder={efektifKlienId ? 'Contoh: Cek backup situs' : 'Contoh: Kirim laporan bulanan'}
          autoComplete="off"
          maxLength={120}
          value={judul}
          data-autofocus=""
          aria-invalid={!!galat.judul}
          aria-describedby={galat.judul ? id('judul-e') : undefined}
          onChange={(e) => {
            setJudul(e.target.value)
            hapusGalat('judul')
          }}
        />
        <FieldError id={id('judul-e')} pesan={galat.judul} />
        {!tugas && efektifKlienId ? (
          <div className="tg-contoh" role="group" aria-label="Contoh tugas">
            {contohTugas.map((c) => (
              <button
                key={c.judul}
                type="button"
                className="tg-contoh__item"
                aria-pressed={judul === c.judul}
                onClick={() => pakaiContoh(c)}
              >
                {c.label}
              </button>
            ))}
          </div>
        ) : null}
      </div>
      <div className="field">
        <span className="field__label" id={id('jadwal-l')}>
          Jadwal
        </span>
        <div className="seg is-full" role="radiogroup" aria-labelledby={id('jadwal-l')}>
          {pilihanJadwal.map((p) => (
            <button
              key={p.label}
              type="button"
              className="seg__item"
              role="radio"
              aria-checked={sekali === p.v}
              tabIndex={sekali === p.v ? 0 : -1}
              data-sekali={p.v ? '1' : '0'}
              onClick={() => {
                setSekali(p.v)
                hapusGalat('ulang_jumlah')
              }}
              onKeyDown={(e) => {
                if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
                e.preventDefault()
                const lain = !p.v
                setSekali(lain)
                hapusGalat('ulang_jumlah')
                const sib = e.currentTarget.parentElement?.querySelector<HTMLElement>(`[data-sekali="${lain ? '1' : '0'}"]`)
                sib?.focus()
              }}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>
      {sekali ? (
        kolomTanggal
      ) : (
        <>
          <div className="field-grid">
            <div className="field">
              <span className="field__label" id={id('ulang-l')}>
                Berulang
              </span>
              <div className="tg-ulang" role="group" aria-labelledby={id('ulang-l')}>
                <span className="tg-ulang__tiap" aria-hidden="true">
                  Tiap
                </span>
                <input
                  className="control num"
                  inputMode="numeric"
                  aria-label="Jumlah"
                  autoComplete="off"
                  value={jumlah}
                  aria-invalid={!!galat.ulang_jumlah}
                  aria-describedby={galat.ulang_jumlah ? id('ulang-e') : undefined}
                  onChange={(e) => {
                    setJumlah(hanyaAngka(e.target.value, 3))
                    hapusGalat('ulang_jumlah')
                  }}
                />
                <Pilihan
                  label="Satuan"
                  nilai={unit}
                  opsi={satuan.map((x) => ({ nilai: x.v, label: x.label }))}
                  onPilih={(v) => setUnit(v)}
                />
              </div>
              <FieldError id={id('ulang-e')} pesan={galat.ulang_jumlah} />
            </div>
            {kolomTanggal}
          </div>
          {ulangValid && !galat.ulang_jumlah && !galat.jatuh_tempo ? (
            <p className="field__hint tg-dialog__hint">
              Setelah ditandai selesai, jatuh tempo maju {n} {unit}.
            </p>
          ) : null}
        </>
      )}
      <div className="field">
        <label className="field__label" htmlFor={id('biaya')}>
          Biaya <span>Opsional</span>
        </label>
        <div className="tg-biaya">
          <div className="seg" role="radiogroup" aria-label="Mata uang">
            {pilihanUang.map((u) => (
              <button
                key={u.v}
                type="button"
                className="seg__item"
                role="radio"
                aria-checked={mataUang === u.v}
                aria-label={u.aria}
                tabIndex={mataUang === u.v ? 0 : -1}
                data-uang={u.v}
                onClick={() => gantiMataUang(u.v)}
                onKeyDown={(e) => {
                  if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
                  e.preventDefault()
                  const lain: MataUang = u.v === 'IDR' ? 'USD' : 'IDR'
                  gantiMataUang(lain)
                  const sib = e.currentTarget.parentElement?.querySelector<HTMLElement>(`[data-uang="${lain}"]`)
                  sib?.focus()
                }}
              >
                {u.label}
              </button>
            ))}
          </div>
          <input
            id={id('biaya')}
            className="control num"
            inputMode={mataUang === 'USD' ? 'decimal' : 'numeric'}
            placeholder={mataUang === 'USD' ? '0.00' : '0'}
            autoComplete="off"
            value={mataUang === 'IDR' ? (biaya ? angka(Number(biaya)) : '') : biaya}
            aria-invalid={!!galat.biaya}
            aria-describedby={galat.biaya ? id('biaya-e') : id('biaya-h')}
            onChange={(e) => {
              setBiaya(mataUang === 'USD' ? hanyaDesimal(e.target.value) : hanyaAngka(e.target.value, 13))
              hapusGalat('biaya')
            }}
          />
        </div>
        {galat.biaya ? (
          <FieldError id={id('biaya-e')} pesan={galat.biaya} />
        ) : (
          <div className="field__hint" id={id('biaya-h')}>
            {sekali
              ? 'Biaya untuk tenggat ini, jika ada.'
              : 'Biaya tiap kali tugas ini dikerjakan, misalnya tagihan perpanjangan.'}
          </div>
        )}
      </div>
      <div className="field">
        <label className="field__label" htmlFor={id('catatan')}>
          Catatan <span>Opsional</span>
        </label>
        <textarea
          id={id('catatan')}
          className="control tg-textarea"
          placeholder="Langkah, akses, atau hal yang perlu diingat"
          rows={2}
          maxLength={500}
          value={catatan}
          aria-invalid={!!galat.catatan}
          aria-describedby={galat.catatan ? id('catatan-e') : undefined}
          onChange={(e) => {
            setCatatan(e.target.value)
            hapusGalat('catatan')
          }}
        />
        <FieldError id={id('catatan-e')} pesan={galat.catatan} />
      </div>
      <Kaki onClose={onClose} sedang={simpan.isPending} label="Simpan" labelSedang="Menyimpan" bisa={judul.trim() !== '' && !simpan.isPending} />
    </Kerangka>
  )
}

interface SelesaiDialogProps {
  tugas: Tugas
  onClose: () => void
  onSaved: (t: Tugas) => void
}

export function SelesaiDialog({ tugas, onClose, onSaved }: SelesaiDialogProps) {
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`
  const toast = useToast()
  const segarkan = useSegarkan()
  const hariIni = hariIniIso()
  const [tgl, setTgl] = useState(hariIni)
  const [menit, setMenit] = useState('')
  const [catatan, setCatatan] = useState('')
  const [galat, setGalat] = useState<Record<string, string>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)

  const selesai = useMutation({
    mutationFn: () =>
      tugasApi.selesai(tugas.id, {
        dari: tugas.jatuh_tempo,
        tanggal: tgl,
        catatan: catatan.trim() || undefined,
        durasi_menit: menit === '' ? undefined : Number(menit),
      }),
    onSuccess: onSaved,
    onError: (err) => {
      if (err instanceof ApiError && err.status === 409) {
        segarkan()
        onClose()
        toast.galat(
          err.message === 'Tugas ini sudah selesai.' ? 'Tugas ini sudah selesai.' : 'Tugas baru saja diubah.',
          'Daftar sudah diperbarui.',
        )
        return
      }
      if (err instanceof ApiError && Object.keys(err.field).length > 0) {
        setGalat(err.field)
        fokusGalat()
      } else setGalatUmum(pesanGalat(err))
    },
  })

  function kirim() {
    const g: Record<string, string> = {}
    if (!tgl) g.tanggal = 'Pilih tanggal tugas dikerjakan.'
    else if (tgl > hariIni) g.tanggal = 'Tanggal tidak boleh setelah hari ini.'
    if (menit !== '' && !(Number(menit) >= 1 && Number(menit) <= 1440)) g.durasi_menit = 'Isi 1 sampai 1.440 menit.'
    if (Object.keys(g).length > 0) {
      setGalat(g)
      fokusGalat()
      return
    }
    setGalatUmum(null)
    selesai.mutate()
  }

  return (
    <Kerangka judul="Tandai selesai" onClose={onClose} onSubmit={kirim}>
      <p className="dialog__text">
        {tugas.sekali ? (
          <>
            <b>{tugas.judul}</b>. Tugas sekali ini akan ditutup dan hilang dari daftar.
          </>
        ) : (
          <>
            <b>{tugas.judul}</b>. Jatuh tempo berikutnya {tanggal(jatuhTempoBerikutnya(tugas))}.
          </>
        )}
      </p>
      <GalatUmum pesan={galatUmum} />
      <div className="field-grid">
        <div className="field">
          <label className="field__label" htmlFor={id('tgl')}>
            Tanggal dikerjakan
          </label>
          <label className="input-icon">
            <Icon name="calendar" />
            <input
              id={id('tgl')}
              type="date"
              className={tgl ? 'control num has-value' : 'control num'}
              value={tgl}
              min="2000-01-01"
              max={hariIni}
              aria-invalid={!!galat.tanggal}
              aria-describedby={galat.tanggal ? id('tgl-e') : undefined}
              onChange={(e) => {
                setTgl(e.target.value)
                setGalat((g) => ({ ...g, tanggal: '' }))
              }}
            />
          </label>
          <FieldError id={id('tgl-e')} pesan={galat.tanggal} />
        </div>
        <div className="field">
          <label className="field__label" htmlFor={id('menit')}>
            Durasi <span>Opsional</span>
          </label>
          <div className="tg-suffix">
            <input
              id={id('menit')}
              className="control num"
              inputMode="numeric"
              placeholder="0"
              autoComplete="off"
              value={menit}
              data-autofocus=""
              aria-invalid={!!galat.durasi_menit}
              aria-describedby={galat.durasi_menit ? id('menit-e') : undefined}
              onChange={(e) => {
                setMenit(hanyaAngka(e.target.value, 4))
                setGalat((g) => ({ ...g, durasi_menit: '' }))
              }}
            />
            <span className="tg-suffix__u" aria-hidden="true">
              menit
            </span>
          </div>
          <FieldError id={id('menit-e')} pesan={galat.durasi_menit} />
        </div>
      </div>
      <div className="field">
        <label className="field__label" htmlFor={id('catatan')}>
          Catatan <span>Opsional</span>
        </label>
        <textarea
          id={id('catatan')}
          className="control tg-textarea"
          placeholder="Apa yang dikerjakan atau ditemukan"
          rows={2}
          maxLength={500}
          value={catatan}
          aria-invalid={!!galat.catatan}
          aria-describedby={galat.catatan ? id('catatan-e') : id('catatan-h')}
          onChange={(e) => {
            setCatatan(e.target.value)
            setGalat((g) => ({ ...g, catatan: '' }))
          }}
        />
        {galat.catatan ? (
          <FieldError id={id('catatan-e')} pesan={galat.catatan} />
        ) : (
          <div className="field__hint" id={id('catatan-h')}>
            {tugas.klien_id
              ? 'Tercatat di aktivitas klien dan masuk ke laporan bulanan.'
              : 'Tercatat di riwayat aktivitas.'}
          </div>
        )}
      </div>
      <Kaki onClose={onClose} sedang={selesai.isPending} label="Tandai selesai" labelSedang="Menyimpan" bisa={!selesai.isPending} />
    </Kerangka>
  )
}
