// "Perlu dikerjakan" on /app: tasks of every client (plus personal deadlines without a client) that are overdue or
// due within 14 days. Client rows open the client's Tugas tab; clientless rows can be completed, edited or deleted here.
import { useMutation, useQuery } from '@tanstack/react-query'
import { useCallback, useId, useState } from 'react'
import { kunciTugas, labelUlang, tugasApi, useSegarkan, type Tugas } from '../lib/api-tugas'
import { sisaHari, tanggal } from '../lib/format'
import { Tautan } from '../lib/rute'
import { ConfirmDialog } from './ConfirmDialog'
import { Icon } from './Icon'
import { Ikon } from './klien/ikon'
import { SelesaiDialog, TugasDialog } from './klien/TugasDialog'
import { Menu } from './RowMenu'
import { pesanGalat, useToast } from './Toasts'
import { Pill } from './ui'
import '../styles/tugas.css'

const RENTANG = 14
const AWAL = 6

type Dialog =
  | { jenis: 'tambah' }
  | { jenis: 'ubah'; tugas: Tugas }
  | { jenis: 'selesai'; tugas: Tugas }
  | { jenis: 'hapus'; tugas: Tugas }

export function PerluDikerjakan() {
  const id = useId()
  const toast = useToast()
  const segarkan = useSegarkan()
  const [semua, setSemua] = useState(false)
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const [menu, setMenu] = useState<{ tugas: Tugas; el: HTMLElement } | null>(null)

  const q = useQuery({ queryKey: kunciTugas.dalamHari(RENTANG), queryFn: () => tugasApi.daftar({ dalamHari: RENTANG }) })
  const list = [...(q.data ?? [])].sort((a, b) => a.jatuh_tempo.localeCompare(b.jatuh_tempo))
  const tampil = semua ? list : list.slice(0, AWAL)
  const lewat = list.filter((t) => t.status === 'lewat').length

  const hapus = useMutation({
    mutationFn: (t: Tugas) => tugasApi.hapus(t.id),
    onSuccess: (_, t) => {
      setDialog(null)
      segarkan()
      toast.ok('Tenggat dihapus.', t.judul)
    },
    onError: (err) => toast.dariGalat('Tenggat tidak dapat dihapus.', err),
  })

  const tutupMenu = useCallback((kembali: boolean) => {
    setMenu((m) => {
      if (kembali) m?.el.focus()
      return null
    })
  }, [])

  let isi
  if (q.isPending) {
    isi = (
      <div className="pd__list" aria-busy="true" aria-label="Memuat tugas">
        {[0, 1, 2].map((i) => (
          <div key={i} className="pd__row is-sk">
            <span className="sk" style={{ width: 64, height: 24 }} />
            <span className="sk" style={{ width: '50%', height: 16 }} />
          </div>
        ))}
      </div>
    )
  } else if (q.isError) {
    isi = (
      <div className="pd__msg" role="alert">
        <span>Tugas tidak dapat dimuat. {pesanGalat(q.error)}</span>
        <button type="button" className="btn btn--secondary is-sm" onClick={() => void q.refetch()}>
          Coba lagi
        </button>
      </div>
    )
  } else if (list.length === 0) {
    isi = (
      <div className="pd__msg">
        <Icon name="check" />
        <span>Tidak ada tugas yang lewat atau jatuh tempo dalam {RENTANG} hari.</span>
      </div>
    )
  } else {
    isi = (
      <>
        <ul className="pd__list">
          {tampil.map((t) => {
            const jadwal = t.sekali ? 'Sekali' : labelUlang(t.ulang_jumlah, t.ulang_satuan)
            const pokok = (
              <>
                <Pill status={t.status} />
                <span className="pd__main">
                  <span className="pd__judul">{t.judul}</span>
                  <span className="pd__sub">
                    <span className="pd__klien">{t.klien || 'Tanpa klien'}</span>
                    <span className="pd__ulang"> · {jadwal}</span>
                  </span>
                </span>
                <span className="pd__due num">
                  <span className={t.sisa_hari < 0 ? 'pd__sisa is-lewat' : 'pd__sisa'}>{sisaHari(t.sisa_hari)}</span>
                  <span className="pd__tgl">{tanggal(t.jatuh_tempo)}</span>
                </span>
              </>
            )
            return (
              <li key={t.id}>
                {t.klien_id ? (
                  <Tautan ke={`/app/klien/${t.klien_id}/tugas`} className="pd__row">
                    {pokok}
                    <Ikon name="chevron-right" className="pd__go" />
                  </Tautan>
                ) : (
                  <div className="pd__row is-pribadi">
                    {pokok}
                    <span className="pd__acts">
                      <button
                        type="button"
                        className="btn btn--secondary is-sm"
                        onClick={() => setDialog({ jenis: 'selesai', tugas: t })}
                        aria-label={`Tandai selesai: ${t.judul}`}
                      >
                        Selesai
                      </button>
                      <button
                        type="button"
                        className="btn btn--ghost is-sm btn--icon"
                        aria-label={`Tindakan lain: ${t.judul}`}
                        aria-haspopup="menu"
                        aria-expanded={menu?.tugas.id === t.id}
                        onClick={(e) => setMenu((m) => (m?.tugas.id === t.id ? null : { tugas: t, el: e.currentTarget }))}
                      >
                        <Icon name="more" />
                      </button>
                    </span>
                  </div>
                )}
              </li>
            )
          })}
        </ul>
        {list.length > AWAL ? (
          <button type="button" className="btn btn--ghost is-sm pd__more" aria-expanded={semua} onClick={() => setSemua((s) => !s)}>
            {semua ? 'Tampilkan lebih sedikit' : `Tampilkan ${list.length - AWAL} lainnya`}
          </button>
        ) : null}
      </>
    )
  }

  return (
    <section className="pd" aria-labelledby={id}>
      <div className="pd__head">
        <div className="pd__head-teks">
          <h2 className="pd__title" id={id}>
            Perlu dikerjakan
          </h2>
          <span className="pd__note num">
            {q.data && list.length > 0
              ? `${list.length} tugas${lewat ? `, ${lewat} lewat jatuh tempo` : ''}`
              : `Tugas lewat atau jatuh tempo dalam ${RENTANG} hari`}
          </span>
        </div>
        <button type="button" className="btn btn--secondary is-sm pd__add" onClick={() => setDialog({ jenis: 'tambah' })}>
          <Icon name="plus" />
          Tambah tenggat
        </button>
      </div>
      {isi}
      {menu ? (
        <Menu
          anchor={menu.el}
          label="Tindakan tenggat"
          onClose={tutupMenu}
          items={[
            {
              label: 'Ubah',
              icon: 'pencil',
              onPilih: () => {
                setMenu(null)
                setDialog({ jenis: 'ubah', tugas: menu.tugas })
              },
            },
            {
              label: 'Hapus',
              icon: 'trash',
              bahaya: true,
              onPilih: () => {
                setMenu(null)
                setDialog({ jenis: 'hapus', tugas: menu.tugas })
              },
            },
          ]}
        />
      ) : null}
      {dialog?.jenis === 'tambah' || dialog?.jenis === 'ubah' ? (
        <TugasDialog
          tugas={dialog.jenis === 'ubah' ? dialog.tugas : undefined}
          awalSekali={dialog.jenis === 'tambah'}
          onClose={() => setDialog(null)}
          onSaved={(t, baru) => {
            setDialog(null)
            segarkan()
            const keterangan =
              baru && !t.klien_id && t.sisa_hari > RENTANG
                ? `${t.judul}, jatuh tempo ${tanggal(t.jatuh_tempo)} (tampil ${RENTANG} hari sebelum jatuh tempo).`
                : `${t.judul}, jatuh tempo ${tanggal(t.jatuh_tempo)}.`
            toast.ok(baru ? 'Tenggat ditambahkan.' : 'Tenggat disimpan.', keterangan)
          }}
        />
      ) : null}
      {dialog?.jenis === 'selesai' ? (
        <SelesaiDialog
          tugas={dialog.tugas}
          onClose={() => setDialog(null)}
          onSaved={(t) => {
            setDialog(null)
            segarkan()
            if (t.sekali) toast.ok('Selesai.', t.judul)
            else toast.ok('Tugas selesai.', `Jatuh tempo berikutnya ${tanggal(t.jatuh_tempo)}.`)
          }}
        />
      ) : null}
      {dialog?.jenis === 'hapus' ? (
        <ConfirmDialog
          judul="Hapus tenggat?"
          tombol="Hapus"
          tombolSedang="Menghapus"
          sedang={hapus.isPending}
          onBatal={() => setDialog(null)}
          onYa={() => hapus.mutate(dialog.tugas)}
        >
          <b>{dialog.tugas.judul}</b> dihapus dari daftar.
        </ConfirmDialog>
      ) : null}
    </section>
  )
}
