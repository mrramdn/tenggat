// Read-only link to a client's reports. Same interaction as the calendar link (KalenderDialog): the API shows the
// link only once, when it is made, so a link made earlier can only be replaced or revoked.
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { useModal } from '../../hooks/useModal'
import { KUNCI_KLIEN } from '../../lib/api'
import { kunciLaporan, labelBulan, laporanApi } from '../../lib/api-laporan'
import { Icon } from '../Icon'
import { useToast } from '../Toasts'

type Konfirmasi = null | 'ganti' | 'cabut'

interface Props {
  klienId: string
  bulan: string
  /** laporan_aktif from the client list; undefined while unknown. */
  aktif: boolean | undefined
  onClose: () => void
}

export function BagikanLaporanDialog({ klienId, bulan, aktif, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  useModal(ref, onClose)
  const uid = useId()
  const qc = useQueryClient()
  const toast = useToast()
  const kunciJalur = kunciLaporan.jalur(klienId)

  const [jalur, setJalurState] = useState<string | null>(() => qc.getQueryData<string | null>(kunciJalur) ?? null)
  const [konfirmasi, setKonfirmasi] = useState<Konfirmasi>(null)
  const [tersalin, setTersalin] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  function setJalur(j: string | null) {
    qc.setQueryData(kunciJalur, j)
    setJalurState(j)
  }

  const segarkan = () => {
    void qc.invalidateQueries({ queryKey: KUNCI_KLIEN })
    void qc.invalidateQueries({ queryKey: ['aktivitas'] })
  }

  const buat = useMutation({
    mutationFn: () => laporanApi.buatTautan(klienId),
    onSuccess: (j) => {
      const ganti = aktif === true || jalur != null
      setJalur(j)
      setKonfirmasi(null)
      setTersalin(false)
      toast.ok(ganti ? 'Tautan laporan diganti.' : 'Tautan laporan dibuat.', ganti ? 'Tautan lama tidak berlaku lagi.' : undefined)
    },
    onError: (err) => toast.dariGalat('Tautan laporan tidak dapat dibuat.', err),
    onSettled: segarkan,
  })

  const cabut = useMutation({
    mutationFn: () => laporanApi.cabutTautan(klienId),
    onSuccess: () => {
      setJalur(null)
      setKonfirmasi(null)
      toast.ok('Tautan laporan dicabut.', 'Klien tidak dapat lagi membuka laporan lewat tautan itu.')
    },
    onError: (err) => toast.dariGalat('Tautan laporan tidak dapat dicabut.', err),
    onSettled: segarkan,
  })

  // A link made here but revoked elsewhere is no longer valid.
  useEffect(() => {
    if (aktif === false && jalur && !buat.isPending) setJalur(null)
  }, [aktif])

  useEffect(() => {
    if (!tersalin) return
    const t = window.setTimeout(() => setTersalin(false), 2000)
    return () => window.clearTimeout(t)
  }, [tersalin])

  const url = jalur ? `${window.location.origin}${jalur}?bulan=${bulan}` : ''

  async function salin() {
    try {
      await navigator.clipboard.writeText(url)
      setTersalin(true)
    } catch {
      inputRef.current?.select()
      toast.galat('Tautan tidak dapat disalin otomatis.', 'Tautan sudah dipilih. Salin dengan Ctrl+C atau menu salin.')
    }
  }

  const sibuk = buat.isPending || cabut.isPending

  let isi: ReactNode
  if (aktif === undefined && !jalur) {
    isi = (
      <div aria-busy="true" aria-label="Memuat">
        <span className="sk" style={{ width: '100%', height: 36 }} />
      </div>
    )
  } else if (konfirmasi) {
    isi = (
      <div className="kal__konfirmasi" role="group" aria-labelledby={`${uid}-k`}>
        <p id={`${uid}-k`}>
          {konfirmasi === 'ganti'
            ? 'Tautan lama berhenti bekerja. Klien yang menyimpan tautan lama perlu menerima tautan baru dari Anda.'
            : 'Klien tidak dapat lagi membuka laporan lewat tautan ini. Anda dapat membuat tautan baru kapan saja.'}
        </p>
        <div className="kal__aksi">
          <button type="button" className="btn btn--secondary" onClick={() => setKonfirmasi(null)} disabled={sibuk} data-autofocus="">
            Batal
          </button>
          {konfirmasi === 'ganti' ? (
            <button type="button" className="btn btn--primary" onClick={() => buat.mutate()} disabled={sibuk} aria-busy={buat.isPending}>
              <Icon name={buat.isPending ? 'loader' : 'refresh'} className={buat.isPending ? 'spin' : undefined} />
              {buat.isPending ? 'Mengganti' : 'Ganti tautan'}
            </button>
          ) : (
            <button type="button" className="btn btn--danger" onClick={() => cabut.mutate()} disabled={sibuk} aria-busy={cabut.isPending}>
              <Icon name={cabut.isPending ? 'loader' : 'trash'} className={cabut.isPending ? 'spin' : undefined} />
              {cabut.isPending ? 'Mencabut' : 'Cabut tautan'}
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
            Tautan laporan
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
          <div className="field__hint">
            Tautan ini hanya tampil sekarang. Simpan atau kirim ke klien sebelum menutup jendela ini.
          </div>
        </div>
        <a className="btn btn--secondary kal__buka" href={url} target="_blank" rel="noopener noreferrer">
          <Icon name="external-link" />
          Buka laporan
        </a>
        <div className="kal__kelola">
          <button type="button" className="btn-teks" onClick={() => setKonfirmasi('ganti')}>
            Ganti tautan
          </button>
          <button type="button" className="btn-teks is-danger" onClick={() => setKonfirmasi('cabut')}>
            Cabut tautan
          </button>
        </div>
      </>
    )
  } else if (aktif) {
    isi = (
      <>
        <div className="kal__info">
          <Icon name="info" />
          <span>
            Tautan laporan sudah aktif. Tenggat hanya menampilkannya sekali, saat dibuat. Untuk melihat tautan lagi,
            ganti dengan tautan baru. Tautan lama lalu berhenti bekerja.
          </span>
        </div>
        <div className="kal__aksi">
          <button type="button" className="btn btn--secondary" onClick={() => setKonfirmasi('cabut')}>
            <Icon name="trash" />
            Cabut tautan
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
          {buat.isPending ? 'Membuat tautan' : 'Buat tautan laporan'}
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
            Bagikan laporan
          </h2>
          <button type="button" className="btn btn--ghost btn--icon" aria-label="Tutup" onClick={onClose}>
            <Icon name="x" />
          </button>
        </div>
        <p className="dialog__text" id={`${uid}-t`}>
          Klien membuka laporan {labelBulan(bulan)} lewat tautan ini tanpa masuk, lalu dapat memilih bulan lain di
          halaman itu.
        </p>
        <div className="kal__isi" aria-live="polite">
          {isi}
        </div>
        <div className="note kal__catatan">
          <Icon name="shield" />
          <span>
            Siapa pun yang memegang tautan ini dapat membaca laporan klien ini, termasuk nama aset, tanggal, dan biaya.
            Catatan internal tidak ikut.
          </span>
        </div>
        <div className="dialog__foot">
          <button type="button" className="btn btn--secondary" onClick={onClose}>
            Tutup
          </button>
        </div>
      </div>
    </>
  )
}
