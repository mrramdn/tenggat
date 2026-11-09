// Overflow menu of a row: Ubah and Hapus.
import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { Icon } from './Icon'

interface Props {
  anchor: HTMLElement
  onUbah: () => void
  onHapus: () => void
  onClose: (kembalikanFokus: boolean) => void
}

export function RowMenu({ anchor, onUbah, onHapus, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)

  useLayoutEffect(() => {
    const r = anchor.getBoundingClientRect()
    const el = ref.current
    const h = el?.offsetHeight ?? 80
    const w = el?.offsetWidth ?? 168
    const bawah = r.bottom + 4 + h <= window.innerHeight - 8
    setPos({ top: bawah ? r.bottom + 4 : r.top - 4 - h, left: Math.max(8, r.right - w) })
  }, [anchor])

  // Focus the first item once the menu is placed (it is hidden until then).
  const terpasang = pos !== null
  useEffect(() => {
    if (terpasang) ref.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
  }, [terpasang])

  useEffect(() => {
    const esc = (e: globalThis.KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.preventDefault()
      onClose(true)
    }
    const keluar = (e: Event) => {
      if (ref.current?.contains(e.target as Node) || anchor.contains(e.target as Node)) return
      onClose(false)
    }
    const gulir = () => onClose(false)
    document.addEventListener('pointerdown', keluar)
    document.addEventListener('keydown', esc)
    window.addEventListener('resize', gulir)
    document.addEventListener('scroll', gulir, true)
    return () => {
      document.removeEventListener('pointerdown', keluar)
      document.removeEventListener('keydown', esc)
      window.removeEventListener('resize', gulir)
      document.removeEventListener('scroll', gulir, true)
    }
  }, [anchor, onClose])

  function onKeyDown(e: KeyboardEvent) {
    const items = [...(ref.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])]
    const i = items.indexOf(document.activeElement as HTMLElement)
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      const n = (i + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
      items[n]?.focus()
    } else if (e.key === 'Tab') {
      e.preventDefault()
      onClose(true)
    }
  }

  return (
    <div
      ref={ref}
      className="menu"
      role="menu"
      aria-label="Tindakan aset"
      style={pos ? { top: pos.top, left: pos.left } : { visibility: 'hidden', top: 0, left: 0 }}
      onKeyDown={onKeyDown}
    >
      <button type="button" className="menu__item" role="menuitem" onClick={onUbah}>
        <Icon name="pencil" />
        Ubah
      </button>
      <button type="button" className="menu__item is-danger" role="menuitem" onClick={onHapus}>
        <Icon name="trash" />
        Hapus
      </button>
    </div>
  )
}
