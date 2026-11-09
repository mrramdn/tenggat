// Icons for tasks and the activity feed. Shapes Icon.tsx does not have live here (Lucide, ISC licence), drawn the
// same way, so the shared icon file stays untouched.
import type { JenisAktivitas } from '../../lib/api-tugas'
import { Icon, type IconName } from '../Icon'

const tambahan = {
  'file-text': <><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z" /><path d="M14 2v4a2 2 0 0 0 2 2h4" /><path d="M10 9H8" /><path d="M16 13H8" /><path d="M16 17H8" /></>,
  wrench: <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />,
  'calendar-check': <><rect width="18" height="18" x="3" y="4" rx="2" /><path d="M16 2v4" /><path d="M8 2v4" /><path d="M3 10h18" /><path d="m9 16 2 2 4-4" /></>,
  'list-checks': <><path d="m3 17 2 2 4-4" /><path d="m3 7 2 2 4-4" /><path d="M13 6h8" /><path d="M13 12h8" /><path d="M13 18h8" /></>,
  'circle-plus': <><circle cx="12" cy="12" r="10" /><path d="M8 12h8" /><path d="M12 8v8" /></>,
  'shield-check': <><path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z" /><path d="m9 12 2 2 4-4" /></>,
  'chevron-right': <path d="m9 18 6-6-6-6" />,
  clock: <><circle cx="12" cy="12" r="10" /><path d="M12 6v6l4 2" /></>,
  'circle-check': <><circle cx="12" cy="12" r="10" /><path d="m9 12 2 2 4-4" /></>,
}

export type NamaIkon = IconName | keyof typeof tambahan

export function Ikon({ name, className }: { name: NamaIkon; className?: string }) {
  if (!(name in tambahan)) return <Icon name={name as IconName} className={className} />
  return (
    <svg className={className ? `i ${className}` : 'i'} viewBox="0 0 24 24" aria-hidden="true">
      {tambahan[name as keyof typeof tambahan]}
    </svg>
  )
}

export const ikonAktivitas: Record<JenisAktivitas, NamaIkon> = {
  catatan: 'file-text',
  pekerjaan: 'wrench',
  aset_baru: 'circle-plus',
  aset_diperpanjang: 'calendar-check',
  aset_dihapus: 'trash',
  cek_diperbarui: 'shield-check',
  cek_gagal: 'alert',
  tugas_baru: 'list-checks',
  tugas_selesai: 'circle-check',
  tugas_dihapus: 'trash',
  laporan_dibagikan: 'link',
}
