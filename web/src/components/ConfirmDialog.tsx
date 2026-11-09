// Small confirmation before deleting an asset.
import { useId, useRef } from 'react'
import type { Item } from '../lib/api'
import { useModal } from '../hooks/useModal'
import { Icon } from './Icon'

export function ConfirmDialog({ item, sedang, onBatal, onHapus }: { item: Item; sedang: boolean; onBatal: () => void; onHapus: () => void }) {
  const ref = useRef<HTMLDivElement>(null)
  useModal(ref, onBatal)
  const id = useId()
  return (
    <>
      <div className="scrim" style={{ zIndex: 32 }} onClick={onBatal} aria-hidden="true" />
      <div
        ref={ref}
        className="dialog"
        style={{ zIndex: 33 }}
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${id}-j`}
        aria-describedby={`${id}-t`}
      >
        <h2 className="dialog__title" id={`${id}-j`}>
          Hapus aset ini?
        </h2>
        <p className="dialog__text" id={`${id}-t`}>
          <b>{item.nama}</b> akan dihapus dari daftar dan dari kalender. Tindakan ini tidak dapat dibatalkan.
        </p>
        <div className="dialog__foot">
          <button type="button" className="btn btn--secondary" onClick={onBatal} data-autofocus="">
            Batal
          </button>
          <button type="button" className="btn btn--danger" onClick={onHapus} disabled={sedang} aria-busy={sedang}>
            {sedang ? <Icon name="loader" className="spin" /> : <Icon name="trash" />}
            {sedang ? 'Menghapus' : 'Hapus'}
          </button>
        </div>
      </div>
    </>
  )
}
