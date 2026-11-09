import type { Dispatch, SetStateAction } from 'react'
import type { Akun } from './Akun'
import type { Item, Jenis, Klien } from '../lib/api'
import type { Saring } from '../lib/derive'

export interface BukaTambah {
  jenis?: Jenis
  host?: string
  /** Start "Ambil dari server" right away (empty state). */
  ambil?: boolean
  /** The client chosen at the start (Tambah aset in a client's header). */
  klien?: Pick<Klien, 'id' | 'nama'>
}

/** What the screens can ask the app to do. */
export interface Aksi {
  tambah: (opsi?: BukaTambah) => void
  ubah: (it: Item, fokus?: 'kedaluwarsa') => void
  hapus: (it: Item) => void
  cekUlang: (it: Item) => void
  /** ids of items being re-checked right now */
  sedangCek: ReadonlySet<string>
  tambahKlien: () => void
  ubahKlien: (k: Klien) => void
  hapusKlien: (k: Klien) => void
}

export interface LayarProps {
  items: Item[]
  /** The user's clients from GET /api/klien; empty until loaded. */
  klien: Klien[]
  saring: Saring
  setSaring: Dispatch<SetStateAction<Saring>>
  aksi: Aksi
  akun: Akun
}
