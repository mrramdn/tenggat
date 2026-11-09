import type { Dispatch, SetStateAction } from 'react'
import type { Akun } from './Akun'
import type { Item, Jenis } from '../lib/api'
import type { Saring } from '../lib/derive'

export interface BukaTambah {
  jenis?: Jenis
  host?: string
  /** Start "Ambil dari server" right away (empty state). */
  ambil?: boolean
}

/** What the screens can ask the app to do. */
export interface Aksi {
  tambah: (opsi?: BukaTambah) => void
  ubah: (it: Item, fokus?: 'kedaluwarsa') => void
  hapus: (it: Item) => void
  cekUlang: (it: Item) => void
  /** ids of items being re-checked right now */
  sedangCek: ReadonlySet<string>
}

export interface LayarProps {
  items: Item[]
  saring: Saring
  setSaring: Dispatch<SetStateAction<Saring>>
  aksi: Aksi
  akun: Akun
}
