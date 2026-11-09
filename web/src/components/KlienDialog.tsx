// Dialog "Tambah klien" / "Ubah klien": a name, a short optional note and, when editing, the status.
import { useMutation } from '@tanstack/react-query'
import { useId, useRef, useState } from 'react'
import { ApiError, klienApi, type Klien, type KlienInput, type StatusKlien } from '../lib/api'
import { useModal } from '../hooks/useModal'
import { Icon } from './Icon'
import { pesanGalat } from './Toasts'
import '../styles/klien.css'

interface Props {
  /** The client to edit; without it the dialog adds one. */
  klien?: Klien
  onClose: () => void
  onSaved: (k: Klien, baru: boolean) => void
}

const STATUS: { nilai: StatusKlien; label: string; hint: string }[] = [
  { nilai: 'aktif', label: 'Aktif', hint: 'Klien yang sedang Anda tangani.' },
  { nilai: 'jeda', label: 'Jeda', hint: 'Pekerjaan untuk klien ini berhenti sementara.' },
  { nilai: 'selesai', label: 'Selesai', hint: 'Kerja sama dengan klien ini sudah berakhir.' },
]

export function KlienDialog({ klien, onClose, onSaved }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  useModal(ref, onClose)
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`

  const [nama, setNama] = useState(klien?.nama ?? '')
  const [catatan, setCatatan] = useState(klien?.catatan ?? '')
  // The status field shows only when the API already sends it (it rejects unknown fields otherwise).
  const [status, setStatus] = useState<StatusKlien | undefined>(klien?.status)
  const [galat, setGalat] = useState<Record<string, string>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)

  const simpan = useMutation({
    mutationFn: (input: KlienInput) => (klien ? klienApi.ubah(klien.id, input) : klienApi.buat(input)),
    onSuccess: (k) => onSaved(k, !klien),
    onError: (err) => {
      if (err instanceof ApiError && (err.field.nama || err.field.catatan || err.field.status)) setGalat(err.field)
      else setGalatUmum(pesanGalat(err))
    },
  })

  const bisaSimpan = nama.trim() !== '' && !simpan.isPending

  return (
    <>
      <div className="scrim" style={{ zIndex: 32 }} onClick={onClose} aria-hidden="true" />
      <div ref={ref} className="dialog klien-dialog" style={{ zIndex: 33 }} data-ketik-langsung={klien ? undefined : ''} role="dialog" aria-modal="true" aria-labelledby={id('j')}>
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            if (!bisaSimpan) return
            setGalatUmum(null)
            simpan.mutate({ nama: nama.trim(), catatan: catatan.trim(), ...(status ? { status } : {}) })
          }}
        >
          <h2 className="dialog__title" id={id('j')}>
            {klien ? 'Ubah klien' : 'Tambah klien'}
          </h2>
          {galatUmum ? (
            <div className="form-error" role="alert">
              <Icon name="alert" />
              <span>{galatUmum}</span>
            </div>
          ) : null}
          <div className="field">
            <label className="field__label" htmlFor={id('nama')}>
              Nama
            </label>
            <input
              id={id('nama')}
              className="control"
              placeholder="Nama klien atau usaha"
              autoComplete="off"
              maxLength={80}
              value={nama}
              data-autofocus=""
              aria-invalid={!!galat.nama}
              aria-describedby={galat.nama ? id('nama-e') : undefined}
              onChange={(e) => {
                setNama(e.target.value)
                setGalat((g) => ({ ...g, nama: '' }))
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
            <label className="field__label" htmlFor={id('catatan')}>
              Catatan <span>Opsional</span>
            </label>
            <textarea
              id={id('catatan')}
              className="control klien-dialog__note"
              placeholder="Kontak, cara bayar perpanjangan"
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
              <div className="field__error" id={id('catatan-e')}>
                <Icon name="alert" />
                <span>{galat.catatan}</span>
              </div>
            ) : (
              <div className="field__hint" id={id('catatan-h')}>
                Tampil di halaman klien ini.
              </div>
            )}
          </div>
          {status ? (
            <div className="field">
              <span className="field__label" id={id('status')}>
                Status
              </span>
              <div className={galat.status ? 'seg is-full is-error' : 'seg is-full'} role="radiogroup" aria-labelledby={id('status')} aria-describedby={id('status-h')}>
                {STATUS.map((st) => (
                  <button
                    key={st.nilai}
                    type="button"
                    className="seg__item"
                    role="radio"
                    aria-checked={status === st.nilai}
                    onClick={() => setStatus(st.nilai)}
                  >
                    {st.label}
                  </button>
                ))}
              </div>
              {galat.status ? (
                <div className="field__error">
                  <Icon name="alert" />
                  <span>{galat.status}</span>
                </div>
              ) : (
                <div className="field__hint" id={id('status-h')}>
                  {STATUS.find((st) => st.nilai === status)?.hint}
                </div>
              )}
            </div>
          ) : null}
          <div className="dialog__foot">
            <button type="button" className="btn btn--secondary" onClick={onClose}>
              Batal
            </button>
            <button type="submit" className="btn btn--primary" disabled={!bisaSimpan} aria-busy={simpan.isPending}>
              {simpan.isPending ? <Icon name="loader" className="spin" /> : null}
              {simpan.isPending ? 'Menyimpan' : 'Simpan'}
            </button>
          </div>
        </form>
      </div>
    </>
  )
}
