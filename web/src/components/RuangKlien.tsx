// A client's space (/app/klien/:id[/tab]): the header with name, status, note, figures and actions, then the tabs
// Aset, Tugas, Aktivitas and Laporan, each its own URL.
import type { Item, Klien } from '../lib/api'
import { saringItem, type Saring } from '../lib/derive'
import { rupiahBiasa, tanggal } from '../lib/format'
import { jalurKlien, Tautan, type TabKlien } from '../lib/rute'
import { DaftarAset } from './DaftarAset'
import { DaftarAsetPhone } from './DaftarAsetPhone'
import { Icon } from './Icon'
import { AktivitasKlien } from './klien/AktivitasKlien'
import { LaporanKlien } from './klien/LaporanKlien'
import { TugasKlien } from './klien/TugasKlien'
import type { Aksi } from './types'
import { PillKlien } from './ui'
import '../styles/klien.css'

const LABEL_TAB: Record<TabKlien, string> = { aset: 'Aset', tugas: 'Tugas', aktivitas: 'Aktivitas', laporan: 'Laporan' }
const TAB: TabKlien[] = ['aset', 'tugas', 'aktivitas', 'laporan']

interface Props {
  klien: Klien
  /** Every asset; the space keeps this client's own. */
  items: Item[]
  semuaKlien: Klien[]
  tab: TabKlien
  saring: Saring
  setSaring: (f: (s: Saring) => Saring) => void
  aksi: Aksi
  phone?: boolean
}

function Kepala({ klien: k, aksi, phone }: { klien: Klien; aksi: Aksi; phone: boolean }) {
  return (
    <header className={phone ? 'rk is-phone' : 'rk'}>
      <div className="rk__teks">
        <div className="rk__judul">
          <h1 className="rk__nama">{k.nama}</h1>
          {k.status ? <PillKlien status={k.status} /> : null}
        </div>
        {k.catatan ? <p className="rk__catatan">{k.catatan}</p> : null}
        <ul className="rk__fakta">
          <li>
            <b className="num">{k.jumlah_aset}</b> aset
          </li>
          <li>
            {k.biaya_30_hari > 0 ? (
              <>
                <b className="num">{rupiahBiasa(k.biaya_30_hari)}</b> jatuh tempo dalam 30 hari
              </>
            ) : (
              'Tidak ada biaya dalam 30 hari'
            )}
          </li>
          {k.terdekat ? (
            <li>
              Tenggat terdekat <b className="num">{tanggal(k.terdekat)}</b>
            </li>
          ) : null}
        </ul>
      </div>
      <div className="rk__aksi">
        <button type="button" className="btn btn--primary" onClick={() => aksi.tambah({ klien: k })}>
          <Icon name="plus" />
          Tambah aset
        </button>
        <button type="button" className="btn btn--secondary" onClick={() => aksi.ubahKlien(k)}>
          <Icon name="pencil" />
          Ubah
        </button>
        <button
          type="button"
          className="btn btn--secondary btn--icon"
          aria-label="Hapus klien"
          title="Hapus klien"
          onClick={() => aksi.hapusKlien(k)}
        >
          <Icon name="trash" />
        </button>
      </div>
    </header>
  )
}

export function RuangKlien({ klien: k, items, semuaKlien, tab, saring, setSaring, aksi, phone = false }: Props) {
  const milik = items.filter((it) => it.klien_id === k.id)
  return (
    <div className={phone ? 'ruang is-phone' : 'ruang'}>
      <Kepala klien={k} aksi={aksi} phone={phone} />
      <nav className={phone ? 'seg is-full rk__tabs' : 'tabs'} aria-label={`Bagian ${k.nama}`}>
        {TAB.map((t) => (
          <Tautan
            key={t}
            ke={jalurKlien(k.id, t)}
            className={phone ? 'seg__item' : 'tabs__item'}
            aria-current={t === tab ? 'page' : undefined}
          >
            {LABEL_TAB[t]}
            {t === 'aset' && !phone ? <span className="tabs__count num">{milik.length}</span> : null}
          </Tautan>
        ))}
      </nav>
      <div className={`ruang__isi is-${tab}`}>
        {tab === 'aset' ? (
          phone ? (
            milik.length === 0 ? (
              <div className="ph-empty">
                <b>Klien ini belum punya aset.</b>
                <span>Tambahkan domain, sertifikat, token, atau langganan milik klien ini.</span>
              </div>
            ) : (
              <DaftarAsetPhone rows={saringItem(milik, { ...saring, cari: '', jenis: null, sumber: null, status: null, klien: null })} aksi={aksi} tampilKlien={false} />
            )
          ) : (
            <DaftarAset items={milik} klien={semuaKlien} saring={saring} setSaring={setSaring} aksi={aksi} satuKlien />
          )
        ) : null}
        {tab === 'tugas' ? <TugasKlien klienId={k.id} /> : null}
        {tab === 'aktivitas' ? <AktivitasKlien klienId={k.id} /> : null}
        {tab === 'laporan' ? <LaporanKlien klienId={k.id} /> : null}
      </div>
    </div>
  )
}
