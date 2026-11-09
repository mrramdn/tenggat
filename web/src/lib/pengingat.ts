// Stub (shell branch): fitur-b replaces this with the full reminder text.
import type { Item } from './api'
import { tanggal } from './format'

/** A wa.me link with a short reminder about the asset, for the owner to send to the client. */
export function tautanPengingat(it: Item): string {
  const teks = `Pengingat: ${it.nama} berakhir ${tanggal(it.kedaluwarsa)}.`
  return `https://wa.me/?text=${encodeURIComponent(teks)}`
}
