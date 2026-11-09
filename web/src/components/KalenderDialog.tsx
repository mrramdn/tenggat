// Calendar subscription: a private link a calendar app polls, plus the one-time .ics download. The API shows the
// link only once, when it is made, so a link made earlier in another tab or device can only be replaced.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react'
import { useModal } from '../hooks/useModal'
import { api } from '../lib/api'
import { Icon } from './Icon'
import { useToast } from './Toasts'

const KUNCI_AKTIF = ['kalender', 'aktif'] as const
// The path made on this page, kept in the query cache so the dialog shows it again after closing. It is dropped
// with the other cached data when the user signs out.
const KUNCI_JALUR = ['kalender', 'jalur'] as const

type Konfirmasi = null | 'ganti' | 'hapus'

export function KalenderDialog({ onClose }: { onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null)
  useModal(ref, onClose)
  const uid = useId()
  const qc = useQueryClient()
  const toast = useToast()

  const aktif = useQuery({ queryKey: KUNCI_AKTIF, queryFn: api.kalenderAktif })
  const [jalur, setJalurState] = useState<string | null>(() => qc.getQueryData<string>(KUNCI_JALUR) ?? null)
  const [konfirmasi, setKonfirmasi] = useState<Konfirmasi>(null)
  const [tersalin, setTersalin] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  function setJalur(j: string | null) {
    if (j) qc.setQueryData(KUNCI_JALUR, j)
    else qc.removeQueries({ queryKey: KUNCI_JALUR, exact: true })
    setJalurState(j)
  }

  const buat = useMutation({
    mutationFn: api.buatTautanKalender,
    onSuccess: (j) => {
      const ganti = aktif.data === true
      setJalur(j)
      setKonfirmasi(null)
      setTersalin(false)
      toast.ok(ganti ? 'Tautan kalender diganti.' : 'Tautan kalender dibuat.', ganti ? 'Tautan lama tidak berlaku lagi.' : undefined)
    },
    onError: (err) => toast.dariGalat('Tautan kalender tidak dapat dibuat.', err),
    onSettled: () => qc.invalidateQueries({ queryKey: KUNCI_AKTIF }),
  })

  const hapus = useMutation({
    mutationFn: api.hapusTautanKalender,
    onSuccess: () => {
      setJalur(null)
      setKonfirmasi(null)
      toast.ok('Tautan kalender dihapus.', 'Aplikasi kalender tidak lagi menerima pembaruan.')
    },
    onError: (err) => toast.dariGalat('Tautan kalender tidak dapat dihapus.', err),
    onSettled: () => qc.invalidateQueries({ queryKey: KUNCI_AKTIF }),
  })

  const unduh = useMutation({
    mutationFn: api.unduhKalender,
    onError: (err) => toast.dariGalat('Kalender tidak dapat diunduh.', err),
  })

  // A link made here but turned off elsewhere is no longer valid.
  useEffect(() => {
    if (aktif.data === false && jalur) setJalur(null)
  }, [aktif.data])

  useEffect(() => {
    if (!tersalin) return
    const t = window.setTimeout(() => setTersalin(false), 2000)
    return () => window.clearTimeout(t)
  }, [tersalin])

  const url = jalur ? `${window.location.origin}${jalur}` : ''
  const webcal = jalur ? `webcal://${window.location.host}${jalur}` : ''

  async function salin() {
    try {
      await navigator.clipboard.writeText(url)
      setTersalin(true)
    } catch {
      inputRef.current?.select()
      toast.galat('Tautan tidak dapat disalin otomatis.', 'Tautan sudah dipilih. Salin dengan Ctrl+C atau menu salin.')
    }
  }

  const sibuk = buat.isPending || hapus.isPending

  let isi: ReactNode
  if (aktif.isPending && !jalur) {
    isi = (
      <div className="kal__muat" aria-busy="true" aria-label="Memuat">
        <span className="sk" style={{ width: '100%', height: 36 }} />
      </div>
    )
  } else if (aktif.isError && !jalur) {
    isi = (
      <div className="form-error" role="alert">
        <Icon name="alert" />
        <span>Status tautan tidak dapat dimuat. Tutup lalu buka lagi jendela ini.</span>
      </div>
    )
  } else if (konfirmasi) {
    isi = (
      <div className="kal__konfirmasi" role="group" aria-labelledby={`${uid}-k`}>
        <p id={`${uid}-k`}>
          {konfirmasi === 'ganti'
            ? 'Tautan lama berhenti bekerja. Kalender yang memakai tautan lama perlu ditambahkan lagi dengan tautan baru.'
            : 'Aplikasi kalender berhenti menerima pembaruan. Acara yang sudah masuk tetap tampil sampai Anda menghapus langganannya di aplikasi tersebut.'}
        </p>
        <div className="kal__aksi">
          <button type="button" className="btn btn--secondary" onClick={() => setKonfirmasi(null)} disabled={sibuk}>
            Batal
          </button>
          {konfirmasi === 'ganti' ? (
            <button type="button" className="btn btn--primary" onClick={() => buat.mutate()} disabled={sibuk} aria-busy={buat.isPending}>
              <Icon name={buat.isPending ? 'loader' : 'refresh'} className={buat.isPending ? 'spin' : undefined} />
              {buat.isPending ? 'Mengganti' : 'Ganti tautan'}
            </button>
          ) : (
            <button type="button" className="btn btn--danger" onClick={() => hapus.mutate()} disabled={sibuk} aria-busy={hapus.isPending}>
              <Icon name={hapus.isPending ? 'loader' : 'trash'} className={hapus.isPending ? 'spin' : undefined} />
              {hapus.isPending ? 'Menghapus' : 'Hapus tautan'}
            </button>
          )}
        </div>
      </div>
    )
  } else if (jalur) {
    isi = (
      <>
        <div className="field">
          <label className="field__label" htmlFor={`${uid}-url`}>
            Tautan langganan
          </label>
          <div className="field-row">
            <input
              ref={inputRef}
              id={`${uid}-url`}
              className="control kal__url"
              readOnly
              value={url}
              spellCheck={false}
              onFocus={(e) => e.currentTarget.select()}
            />
            <button type="button" className="btn btn--secondary kal__salin" onClick={() => void salin()} data-autofocus="">
              <Icon name={tersalin ? 'check' : 'copy'} />
              {tersalin ? 'Tersalin' : 'Salin'}
            </button>
          </div>
          <div className="field__hint">Di Google Calendar, buka Kalender lain, pilih Dari URL, lalu tempel tautan ini.</div>
        </div>
        <a className="btn btn--secondary kal__buka" href={webcal}>
          <Icon name="external-link" />
          Buka di aplikasi Kalender
        </a>
        <div className="kal__kelola">
          <button type="button" className="btn-teks" onClick={() => setKonfirmasi('ganti')}>
            Ganti tautan
          </button>
          <button type="button" className="btn-teks is-danger" onClick={() => setKonfirmasi('hapus')}>
            Hapus tautan
          </button>
        </div>
      </>
    )
  } else if (aktif.data) {
    isi = (
      <>
        <div className="kal__info">
          <Icon name="info" />
          <span>
            Tautan langganan sudah aktif. Tenggat hanya menampilkannya sekali, saat dibuat. Untuk melihat tautan lagi,
            ganti dengan tautan baru. Tautan lama lalu berhenti bekerja.
          </span>
        </div>
        <div className="kal__aksi">
          <button type="button" className="btn btn--secondary" onClick={() => setKonfirmasi('hapus')}>
            <Icon name="trash" />
            Hapus tautan
          </button>
          <button type="button" className="btn btn--primary" onClick={() => setKonfirmasi('ganti')} data-autofocus="">
            <Icon name="refresh" />
            Ganti tautan
          </button>
        </div>
      </>
    )
  } else {
    isi = (
      <div className="kal__aksi is-start">
        <button type="button" className="btn btn--primary" onClick={() => buat.mutate()} disabled={sibuk} aria-busy={buat.isPending} data-autofocus="">
          <Icon name={buat.isPending ? 'loader' : 'link'} className={buat.isPending ? 'spin' : undefined} />
          {buat.isPending ? 'Membuat tautan' : 'Buat tautan langganan'}
        </button>
      </div>
    )
  }

  return (
    <>
      <div className="scrim" onClick={onClose} aria-hidden="true" />
      <div
        ref={ref}
        className="dialog kal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${uid}-j`}
        aria-describedby={`${uid}-t`}
        tabIndex={-1}
      >
        <div className="kal__head">
          <h2 className="dialog__title" id={`${uid}-j`}>
            Langganan kalender
          </h2>
          <button type="button" className="btn btn--ghost btn--icon" aria-label="Tutup" onClick={onClose}>
            <Icon name="x" />
          </button>
        </div>
        <p className="dialog__text" id={`${uid}-t`}>
          Aplikasi kalender membaca tanggal berakhir aset Anda dari tautan ini dan memperbaruinya sendiri.
        </p>
        <div className="kal__isi" aria-live="polite">
          {isi}
        </div>
        <div className="note kal__catatan">
          <Icon name="shield" />
          <span>Siapa pun yang memegang tautan ini dapat melihat nama dan tanggal aset Anda.</span>
        </div>
        <div className="kal__foot">
          <button type="button" className="btn btn--ghost" onClick={() => unduh.mutate()} disabled={unduh.isPending} aria-busy={unduh.isPending}>
            <Icon name={unduh.isPending ? 'loader' : 'download'} className={unduh.isPending ? 'spin' : undefined} />
            Unduh sekali (.ics)
          </button>
          <button type="button" className="btn btn--secondary" onClick={onClose}>
            Tutup
          </button>
        </div>
      </div>
    </>
  )
}

type TombolProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'onClick' | 'type'>

/** A button (topbar, sidebar item, phone tab) that opens the calendar dialog. */
export function BukaKalender({ children, ...props }: TombolProps) {
  const [buka, setBuka] = useState(false)
  return (
    <>
      <button type="button" aria-haspopup="dialog" {...props} onClick={() => setBuka(true)}>
        {children}
      </button>
      {buka ? <KalenderDialog onClose={() => setBuka(false)} /> : null}
    </>
  )
}
