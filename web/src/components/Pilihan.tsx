// The one dropdown of the app: a button styled like every other control that opens the same list as the client
// picker (KlienPicker), so filters, forms and pickers look and behave alike. Keyboard: Enter, Space or the arrow
// keys open it; arrows move, Enter or Space choose, Escape or Tab close; Home and End jump; a letter jumps to the
// next option that starts with it.
import { useId, useRef, useState, type ReactNode } from 'react'
import { Icon } from './Icon'
import '../styles/klien.css'

export interface OpsiPilihan<T extends string> {
  nilai: T
  label: string
  /** Quiet text on the right, e.g. a count. */
  meta?: ReactNode
  /** Shown before the label, e.g. a status dot. */
  awal?: ReactNode
}

interface Props<T extends string> {
  id?: string
  /** Accessible name when there is no visible <label for>. */
  label?: string
  nilai: T
  opsi: OpsiPilihan<T>[]
  onPilih: (v: T) => void
  disabled?: boolean
  className?: string
  invalid?: boolean
  describedBy?: string
}

export function Pilihan<T extends string>({ id, label, nilai, opsi, onPilih, disabled, className, invalid, describedBy }: Props<T>) {
  const listId = useId()
  const btnRef = useRef<HTMLButtonElement>(null)
  const [buka, setBuka] = useState(false)
  const [aktif, setAktif] = useState(0)
  const dipilih = Math.max(0, opsi.findIndex((o) => o.nilai === nilai))
  const sorot = Math.min(aktif, opsi.length - 1)
  const optId = (i: number) => `${listId}-o${i}`

  function bukaDaftar(i = dipilih) {
    if (disabled) return
    setAktif(i)
    setBuka(true)
  }
  function pilih(i: number) {
    const o = opsi[i]
    setBuka(false)
    if (o && o.nilai !== nilai) onPilih(o.nilai)
  }

  return (
    <div className={className ? `picker ${className}` : 'picker'}>
      <button
        ref={btnRef}
        id={id}
        type="button"
        className="control is-select"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={buka}
        aria-controls={listId}
        aria-activedescendant={buka ? optId(sorot) : undefined}
        aria-label={label}
        aria-invalid={invalid || undefined}
        aria-describedby={describedBy}
        disabled={disabled}
        onClick={() => (buka ? setBuka(false) : bukaDaftar())}
        onBlur={() => setBuka(false)}
        onKeyDown={(e) => {
          const k = e.key
          if (!buka) {
            if (k === 'ArrowDown' || k === 'ArrowUp' || k === 'Enter' || k === ' ') {
              e.preventDefault()
              bukaDaftar()
            }
            return
          }
          if (k === 'ArrowDown' || k === 'ArrowUp') {
            e.preventDefault()
            const d = k === 'ArrowDown' ? 1 : -1
            setAktif((sorot + d + opsi.length) % opsi.length)
          } else if (k === 'Home' || k === 'End') {
            e.preventDefault()
            setAktif(k === 'Home' ? 0 : opsi.length - 1)
          } else if (k === 'Enter' || k === ' ') {
            e.preventDefault()
            pilih(sorot)
          } else if (k === 'Escape') {
            e.preventDefault()
            e.stopPropagation()
            setBuka(false)
          } else if (k === 'Tab') {
            setBuka(false)
          } else if (k.length === 1 && /\S/.test(k)) {
            const huruf = k.toLocaleLowerCase('id-ID')
            for (let n = 1; n <= opsi.length; n++) {
              const i = (sorot + n) % opsi.length
              if (opsi[i].label.toLocaleLowerCase('id-ID').startsWith(huruf)) {
                setAktif(i)
                break
              }
            }
          }
        }}
      >
        {opsi[dipilih]?.awal}
        <span className="control__teks">{opsi[dipilih]?.label ?? ''}</span>
      </button>
      {buka ? (
        <ul className="picker__list" id={listId} role="listbox" aria-label={label}>
          {opsi.map((o, i) => (
            <li
              key={o.nilai}
              id={optId(i)}
              role="option"
              aria-selected={i === dipilih}
              className={i === sorot ? 'picker__opt is-active' : 'picker__opt'}
              onMouseDown={(e) => e.preventDefault()}
              onMouseEnter={() => setAktif(i)}
              onClick={() => pilih(i)}
            >
              {o.awal}
              <span className="picker__name">{o.label}</span>
              {o.meta !== undefined ? <span className="picker__meta num">{o.meta}</span> : null}
              <Icon name="tick" className={i === dipilih ? 'picker__check' : 'picker__check is-hidden'} />
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
