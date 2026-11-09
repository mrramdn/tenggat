// Tab "Tugas" of a client's space: recurring maintenance tasks, nearest due date first, with Tandai selesai.
import { useMutation, useQuery } from '@tanstack/react-query'
import { useCallback, useMemo, useState } from 'react'
import { kunciTugas, labelUlang, tugasApi, useSegarkan, type Tugas } from '../../lib/api-tugas'
import { frasaHari, rupiahBiasa, tanggal } from '../../lib/format'
import { ConfirmDialog } from '../ConfirmDialog'
import { Icon } from '../Icon'
import { Menu } from '../RowMenu'
import { pesanGalat, useToast } from '../Toasts'
import { Days, Pill } from '../ui'
import { contohTersisa, type ContohTugas } from './contoh-tugas'
import { Ikon } from './ikon'
import { SelesaiDialog, TugasDialog } from './TugasDialog'
import '../../styles/tugas.css'

type Dialog =
  | { jenis: 'tambah'; contoh?: ContohTugas }
  | { jenis: 'ubah'; tugas: Tugas }
  | { jenis: 'selesai'; tugas: Tugas }
  | { jenis: 'hapus'; tugas: Tugas }

const urut = (a: Tugas, b: Tugas) => a.jatuh_tempo.localeCompare(b.jatuh_tempo) || a.judul.localeCompare(b.judul, 'id')

const tanggalWaktu = (ts: string) => {
  const d = new Date(ts)
  const p = (n: number) => String(n).padStart(2, '0')
  return tanggal(`${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`)
}

function ringkasan(list: Tugas[]): string[] {
  const lewat = list.filter((t) => t.status === 'lewat').length
  const dekat = list.filter((t) => t.status === 'kritis').length
  const bagian = [`${list.length} tugas`]
  if (lewat) bagian.push(`${lewat} lewat jatuh tempo`)
  if (dekat) bagian.push(`${dekat} dalam 7 hari`)
  return bagian
}

export function TugasKlien({ klienId }: { klienId: string }) {
  const toast = useToast()
  const segarkan = useSegarkan()
  const q = useQuery({ queryKey: kunciTugas.klien(klienId), queryFn: () => tugasApi.daftar({ klienId }) })
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const [menu, setMenu] = useState<{ tugas: Tugas; el: HTMLElement } | null>(null)

  const list = useMemo(() => [...(q.data ?? [])].sort(urut), [q.data])
  const saran = useMemo(() => contohTersisa(list.map((t) => t.judul)), [list])

  const hapus = useMutation({
    mutationFn: (t: Tugas) => tugasApi.hapus(t.id),
    onSuccess: (_, t) => {
      setDialog(null)
      segarkan()
      toast.ok('Tugas dihapus', t.judul)
    },
    onError: (err) => toast.dariGalat('Tugas tidak terhapus', err),
  })

  const tutupMenu = useCallback((kembali: boolean) => {
    setMenu((m) => {
      if (kembali) m?.el.focus()
      return null
    })
  }, [])

  const tambah = (contoh?: ContohTugas) => setDialog({ jenis: 'tambah', contoh })

  let isi
  if (q.isPending) {
    isi = (
      <div className="rows" aria-busy="true" aria-label="Memuat tugas">
        {[0, 1, 2].map((i) => (
          <div key={i} className="tg-row">
            <span className="sk" style={{ width: 96, height: 24 }} />
            <span className="sk" style={{ width: '60%', height: 16 }} />
          </div>
        ))}
      </div>
    )
  } else if (q.isError) {
    isi = (
      <div className="tg-galat" role="alert">
        <b>Tugas tidak dapat dimuat.</b>
        <span>{pesanGalat(q.error)}</span>
        <button type="button" className="btn btn--secondary" onClick={() => void q.refetch()}>
          <Icon name="refresh" />
          Coba lagi
        </button>
      </div>
    )
  } else if (list.length === 0) {
    isi = (
      <div className="tg-empty">
        <span className="tg-empty__icon" aria-hidden="true">
          <Ikon name="list-checks" />
        </span>
        <h3 className="tg-empty__title">Belum ada tugas untuk klien ini.</h3>
        <p className="tg-empty__text">
          Tugas adalah pekerjaan rutin untuk klien, misalnya cek backup atau update plugin. Setelah Anda menandainya
          selesai, jatuh temponya maju ke periode berikutnya dan pekerjaannya masuk ke laporan bulanan.
        </p>
        <button type="button" className="btn btn--primary" onClick={() => tambah()}>
          <Icon name="plus" />
          Tambah tugas
        </button>
        <Saran saran={saran} onPilih={tambah} />
      </div>
    )
  } else {
    isi = (
      <>
        <div className="rows tg-rows">
          <div className="tg-row row--head" aria-hidden="true">
            <span className="sisa">
              <span>Sisa</span>
              <span>Status</span>
            </span>
            <span>Tugas</span>
            <span className="col-date">Jatuh tempo</span>
            <span className="col-last">Terakhir selesai</span>
            <span />
          </div>
          <div role="list" aria-label="Tugas klien">
            {list.map((t) => (
              <Baris
                key={t.id}
                t={t}
                menuTerbuka={menu?.tugas.id === t.id}
                onSelesai={() => setDialog({ jenis: 'selesai', tugas: t })}
                bukaMenu={(el) => setMenu((m) => (m?.tugas.id === t.id ? null : { tugas: t, el }))}
              />
            ))}
          </div>
        </div>
        <Saran saran={saran} onPilih={tambah} />
      </>
    )
  }

  return (
    <section className="tg" aria-label="Tugas">
      {list.length > 0 ? (
        <div className="tg-head">
          <ul className="tg-head__sum num" aria-live="polite">
            {ringkasan(list).map((b) => (
              <li key={b}>{b}</li>
            ))}
          </ul>
          <button type="button" className="btn btn--secondary" onClick={() => tambah()}>
            <Icon name="plus" />
            Tambah tugas
          </button>
        </div>
      ) : null}
      {isi}
      {menu ? (
        <Menu
          anchor={menu.el}
          label="Tindakan tugas"
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
          klienId={klienId}
          tugas={dialog.jenis === 'ubah' ? dialog.tugas : undefined}
          contoh={dialog.jenis === 'tambah' ? dialog.contoh : undefined}
          onClose={() => setDialog(null)}
          onSaved={(t, baru) => {
            setDialog(null)
            segarkan()
            toast.ok(baru ? 'Tugas ditambahkan' : 'Tugas disimpan', `${t.judul}, jatuh tempo ${tanggal(t.jatuh_tempo)}.`)
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
            toast.ok('Tugas selesai', `Jatuh tempo berikutnya ${tanggal(t.jatuh_tempo)}.`)
          }}
        />
      ) : null}
      {dialog?.jenis === 'hapus' ? (
        <ConfirmDialog
          judul="Hapus tugas?"
          tombol="Hapus tugas"
          tombolSedang="Menghapus"
          sedang={hapus.isPending}
          onBatal={() => setDialog(null)}
          onYa={() => hapus.mutate(dialog.tugas)}
        >
          <b>{dialog.tugas.judul}</b> dihapus dari daftar tugas. Aktivitas yang sudah tercatat tetap ada.
        </ConfirmDialog>
      ) : null}
    </section>
  )
}

function Baris({ t, menuTerbuka, onSelesai, bukaMenu }: { t: Tugas; menuTerbuka: boolean; onSelesai: () => void; bukaMenu: (el: HTMLElement) => void }) {
  const terakhir = t.terakhir_selesai ? tanggalWaktu(t.terakhir_selesai) : null
  return (
    <div className="tg-row" role="listitem" aria-label={`${t.judul}, ${frasaHari(t.sisa_hari).replace('Berakhir', 'Jatuh tempo')}`}>
      <div className="sisa">
        <Days n={t.sisa_hari} />
        <Pill status={t.status} />
      </div>
      <div className="tg-main">
        <div className="asset__name">{t.judul}</div>
        <div className="asset__sub">
          <Icon name="repeat" />
          <span>{labelUlang(t.ulang_jumlah, t.ulang_satuan)}</span>
          <span className="tg-sub-tgl tg-dot num">{tanggal(t.jatuh_tempo)}</span>
          {t.biaya != null ? <span className="tg-dot num">{rupiahBiasa(t.biaya)}</span> : null}
        </div>
        {t.catatan ? <div className="tg-note">{t.catatan}</div> : null}
        <div className="tg-sub-last">{terakhir ? `Terakhir selesai ${terakhir}` : 'Belum pernah ditandai selesai'}</div>
      </div>
      <div className="cell col-date num">{tanggal(t.jatuh_tempo)}</div>
      <div className={terakhir ? 'cell col-last num' : 'cell col-last tg-belum'}>{terakhir ?? 'Belum pernah'}</div>
      <div className="actions">
        <button type="button" className="btn btn--secondary is-sm btn--fixed tg-selesai" onClick={onSelesai} aria-label={`Tandai selesai: ${t.judul}`}>
          Tandai selesai
        </button>
        <button
          type="button"
          className="btn btn--ghost is-sm btn--icon"
          aria-label={`Tindakan lain: ${t.judul}`}
          aria-haspopup="menu"
          aria-expanded={menuTerbuka}
          onClick={(e) => bukaMenu(e.currentTarget)}
        >
          <Icon name="more" />
        </button>
      </div>
    </div>
  )
}

function Saran({ saran, onPilih }: { saran: ContohTugas[]; onPilih: (c: ContohTugas) => void }) {
  if (saran.length === 0) return null
  return (
    <div className="tg-saran" role="group" aria-label="Contoh tugas">
      <span className="tg-saran__label">Contoh</span>
      {saran.map((c) => (
        <button key={c.judul} type="button" className="tg-contoh__item" onClick={() => onPilih(c)} title={`${c.judul}, ${labelUlang(c.ulang_jumlah, c.ulang_satuan).toLowerCase()}`}>
          <Icon name="plus" />
          {c.label}
          <span className="tg-contoh__ulang">{labelUlang(c.ulang_jumlah, c.ulang_satuan).toLowerCase()}</span>
        </button>
      ))}
    </div>
  )
}
