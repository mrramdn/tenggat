// The signed-in account: a button with the initials, name and email at the bottom of the sidebar, opening a menu
// with Pengaturan and Keluar. On the phone the account lives in the Pengaturan tab.
import { useCallback, useState } from 'react'
import type { Pengguna } from '../lib/api'
import { pergi } from '../lib/rute'
import { Icon } from './Icon'
import { Menu } from './RowMenu'

/** What the screens get to show and act on the account. */
export interface Akun {
  pengguna: Pengguna
  keluar: () => void
  /** Opens the confirmation for deleting the account. */
  hapus: () => void
  sedangKeluar: boolean
}

/** Up to two initials: first and last word of the name. */
export function inisial(nama: string): string {
  const kata = nama.trim().split(/\s+/).filter(Boolean)
  if (kata.length === 0) return '?'
  const awal = (k: string) => Array.from(k)[0] ?? ''
  const s = kata.length > 1 ? awal(kata[0]) + awal(kata[kata.length - 1]) : Array.from(kata[0]).slice(0, 2).join('')
  return s.toLocaleUpperCase('id-ID')
}

function useMenuAkun() {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null)
  const tutup = useCallback((kembali: boolean) => {
    setAnchor((a) => {
      if (a && kembali) a.focus()
      return null
    })
  }, [])
  const alih = useCallback((el: HTMLElement) => setAnchor((a) => (a ? null : el)), [])
  return { anchor, tutup, alih }
}

function MenuAkun({ akun, anchor, tutup }: { akun: Akun; anchor: HTMLElement; tutup: (kembali: boolean) => void }) {
  return (
    <Menu
      anchor={anchor}
      label="Akun"
      selebarAnchor
      onClose={tutup}
      items={[
        {
          label: 'Pengaturan',
          icon: 'settings',
          onPilih: () => {
            tutup(false)
            pergi('/app/pengaturan')
          },
        },
        {
          label: akun.sedangKeluar ? 'Sedang keluar' : 'Keluar',
          icon: akun.sedangKeluar ? 'loader' : 'log-out',
          onPilih: () => {
            if (akun.sedangKeluar) return
            tutup(false)
            akun.keluar()
          },
        },
      ]}
    />
  )
}

export function AkunSidebar({ akun }: { akun: Akun }) {
  const { anchor, tutup, alih } = useMenuAkun()
  const { pengguna } = akun
  return (
    <>
      <button
        type="button"
        className="akun"
        aria-haspopup="menu"
        aria-expanded={!!anchor}
        aria-label={`Akun: ${pengguna.nama}, ${pengguna.email}`}
        title={pengguna.email}
        onClick={(e) => alih(e.currentTarget)}
      >
        <span className="avatar" aria-hidden="true">
          {inisial(pengguna.nama)}
        </span>
        <span className="akun__teks">
          <span className="akun__nama">{pengguna.nama}</span>
          <span className="akun__email">{pengguna.email}</span>
        </span>
        <Icon name="chevrons-up-down" />
      </button>
      {anchor ? <MenuAkun akun={akun} anchor={anchor} tutup={tutup} /> : null}
    </>
  )
}
