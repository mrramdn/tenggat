// Starting points for a client's tasks: what a typical Indonesian website maintenance package covers each month,
// each quarter and each year.
import type { SatuanUlang } from '../../lib/api-tugas'

export interface ContohTugas {
  /** Short label on the suggestion button. */
  label: string
  judul: string
  ulang_jumlah: number
  ulang_satuan: SatuanUlang
}

export const contohTugas: ContohTugas[] = [
  { label: 'Cek backup', judul: 'Cek backup situs', ulang_jumlah: 1, ulang_satuan: 'bulan' },
  { label: 'Update plugin dan core', judul: 'Update plugin, tema, dan core', ulang_jumlah: 1, ulang_satuan: 'bulan' },
  { label: 'Pindai malware', judul: 'Pindai malware', ulang_jumlah: 1, ulang_satuan: 'bulan' },
  { label: 'Tinjau uptime dan SSL', judul: 'Tinjau uptime dan sertifikat SSL', ulang_jumlah: 3, ulang_satuan: 'bulan' },
  { label: 'Tagih perpanjangan', judul: 'Tagih perpanjangan hosting dan domain', ulang_jumlah: 1, ulang_satuan: 'tahun' },
]

/** Suggestions whose title the client does not have yet. */
export function contohTersisa(judulAda: string[]): ContohTugas[] {
  const ada = new Set(judulAda.map((j) => j.trim().toLowerCase()))
  return contohTugas.filter((c) => !ada.has(c.judul.toLowerCase()))
}
