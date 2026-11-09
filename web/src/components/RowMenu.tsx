// Popover menus: the overflow menu of a row (Ubah, WhatsApp reminder, Hapus) and the account menu.
import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { Icon, type IconName } from './Icon'

export interface ItemMenu {
  label: string
  icon: IconName
  bahaya?: boolean
  onPilih: () => void
}

interface MenuProps {
  anchor: HTMLElement
  label: string
  items: ItemMenu[]
  /** Optional block above the items (the account menu shows who is signed in). */
  kepala?: ReactNode
  /** Extra class on the menu, e.g. a wider account menu on the phone. */
  className?: string
  /** Make the menu at least as wide as its anchor (the sidebar account button). */
  selebarAnchor?: boolean
  onClose: (kembalikanFokus: boolean) => void
}

export function RowMenu({
  anchor,
  onUbah,
  onPengingat,
  onHapus,
  onClose,
}: {
  anchor: HTMLElement
  onUbah: () => void
  /** Opens a WhatsApp message to the client about this asset. */
  onPengingat: () => void
  onHapus: () => void
  onClose: (kembalikanFokus: boolean) => void
}) {
  return (
    <Menu
      anchor={anchor}
      label="Tindakan aset"
      onClose={onClose}
      items={[
        { label: 'Ubah', icon: 'pencil', onPilih: onUbah },
        { label: 'Kirim pengingat lewat WhatsApp', icon: 'message-circle', onPilih: onPengingat },
        { label: 'Hapus', icon: 'trash', bahaya: true, onPilih: onHapus },
      ]}
    />
  )
}

export function Menu({ anchor, label, items, kepala, className, selebarAnchor = false, onClose }: MenuProps) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ top: number; left: number; minWidth?: number } | null>(null)

  useLayoutEffect(() => {
    const r = anchor.getBoundingClientRect()
    const el = ref.current
    const h = el?.offsetHeight ?? 80
    const w = Math.max(el?.offsetWidth ?? 168, selebarAnchor ? r.width : 0)
    const bawah = r.bottom + 4 + h <= window.innerHeight - 8
    setPos({ top: bawah ? r.bottom + 4 : r.top - 4 - h, left: Math.max(8, r.right - w), minWidth: selebarAnchor ? r.width : undefined })
  }, [anchor, selebarAnchor])

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
      className={className ? `menu ${className}` : 'menu'}
      role="menu"
      aria-label={label}
      style={pos ? pos : { visibility: 'hidden', top: 0, left: 0 }}
      onKeyDown={onKeyDown}
    >
      {kepala ? <div className="menu__head">{kepala}</div> : null}
      {items.map((it) => (
        <button key={it.label} type="button" className={it.bahaya ? 'menu__item is-danger' : 'menu__item'} role="menuitem" onClick={it.onPilih}>
          <Icon name={it.icon} />
          {it.label}
        </button>
      ))}
    </div>
  )
}
