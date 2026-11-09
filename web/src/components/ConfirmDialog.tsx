// Small confirmation before a destructive action (deleting an asset or the account).
import { useId, useRef, type ReactNode } from 'react'
import { useModal } from '../hooks/useModal'
import { Icon } from './Icon'

interface Props {
  judul: string
  children: ReactNode
  /** Label of the danger button, and its label while the request runs. */
  tombol: string
  tombolSedang: string
  sedang: boolean
  onBatal: () => void
  onYa: () => void
}

export function ConfirmDialog({ judul, children, tombol, tombolSedang, sedang, onBatal, onYa }: Props) {
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
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={`${id}-j`}
        aria-describedby={`${id}-t`}
      >
        <h2 className="dialog__title" id={`${id}-j`}>
          {judul}
        </h2>
        <p className="dialog__text" id={`${id}-t`}>
          {children}
        </p>
        <div className="dialog__foot">
          <button type="button" className="btn btn--secondary" onClick={onBatal} data-autofocus="">
            Batal
          </button>
          <button type="button" className="btn btn--danger" onClick={onYa} disabled={sedang} aria-busy={sedang}>
            {sedang ? <Icon name="loader" className="spin" /> : <Icon name="trash" />}
            {sedang ? tombolSedang : tombol}
          </button>
        </div>
      </div>
    </>
  )
}
