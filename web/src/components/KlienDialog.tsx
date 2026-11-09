// Dialog "Tambah klien" / "Ubah klien": a name and a short optional note.
import { useMutation } from '@tanstack/react-query'
import { useId, useRef, useState } from 'react'
import { ApiError, klienApi, type Klien, type KlienInput } from '../lib/api'
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

export function KlienDialog({ klien, onClose, onSaved }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  useModal(ref, onClose)
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`

  const [nama, setNama] = useState(klien?.nama ?? '')
  const [catatan, setCatatan] = useState(klien?.catatan ?? '')
  const [galat, setGalat] = useState<Record<string, string>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)

  const simpan = useMutation({
    mutationFn: (input: KlienInput) => (klien ? klienApi.ubah(klien.id, input) : klienApi.buat(input)),
    onSuccess: (k) => onSaved(k, !klien),
    onError: (err) => {
      if (err instanceof ApiError && (err.field.nama || err.field.catatan)) setGalat(err.field)
      else setGalatUmum(pesanGalat(err))
    },
  })

  const bisaSimpan = nama.trim() !== '' && !simpan.isPending

  return (
    <>
      <div className="scrim" style={{ zIndex: 32 }} onClick={onClose} aria-hidden="true" />
      <div ref={ref} className="dialog klien-dialog" style={{ zIndex: 33 }} role="dialog" aria-modal="true" aria-labelledby={id('j')}>
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            if (!bisaSimpan) return
            setGalatUmum(null)
            simpan.mutate({ nama: nama.trim(), catatan: catatan.trim() })
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
                Tampil di atas daftar aset klien ini.
              </div>
            )}
          </div>
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
