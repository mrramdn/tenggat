// Phone shell: top bar, the page of the route, and the tab bar (Ringkasan, Klien, Pengaturan). The Klien tab opens
// a sheet with the client list; each client opens its space.
import { useRef, useState } from 'react'
import type { Klien } from '../lib/api'
import { biaya30, judul, saringAwal, saringItem, statusTerburuk, tanpaSaring } from '../lib/derive'
import { hariIniIso, kelasStatus, labelStatus, rupiahBiasa, tanggalPanjang, urutanStatus } from '../lib/format'
import { jalurKlien, Tautan } from '../lib/rute'
import { useModal } from '../hooks/useModal'
import { DaftarAsetPhone } from './DaftarAsetPhone'
import { Galat, TidakAda, type ShellProps } from './Desktop'
import { EksporCsv } from './EksporCsv'
import { EmptyState } from './EmptyState'
import { Icon } from './Icon'
import { Pengaturan } from './Pengaturan'
import { PerluDikerjakan } from './PerluDikerjakan'
import { RuangKlien } from './RuangKlien'
import type { Aksi, LayarProps } from './types'

function SheetKlien({ klien, terburuk, aktifId, aksi, onClose }: { klien: Klien[]; terburuk: ReturnType<typeof statusTerburuk>; aktifId: string | null; aksi: Aksi; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null)
  useModal(ref, onClose)
  return (
    <>
      <div className="scrim" onClick={onClose} aria-hidden="true" />
      <div ref={ref} className="ph-sheet" role="dialog" aria-modal="true" aria-labelledby="ph-sheet-j" tabIndex={-1}>
        <div className="ph-sheet__head">
          <h2 className="ph-sheet__judul" id="ph-sheet-j">
            Klien
          </h2>
          <button type="button" className="btn btn--ghost btn--icon" aria-label="Tutup" onClick={onClose}>
            <Icon name="x" />
          </button>
        </div>
        <div className="ph-sheet__isi">
          {klien.length === 0 ? <p className="ph-sheet__kosong">Belum ada klien.</p> : null}
          {klien.map((k) => {
            const st = terburuk.get(k.id)
            return (
              <Tautan
                key={k.id}
                ke={jalurKlien(k.id)}
                className={k.status === 'jeda' || k.status === 'selesai' ? 'ph-klien is-pasif' : 'ph-klien'}
                aria-current={aktifId === k.id ? 'page' : undefined}
                onClick={onClose}
              >
                <span
                  className={st ? `nav__dot st-${kelasStatus[st]}` : 'nav__dot is-kosong'}
                  role="img"
                  aria-label={st ? `Status terdekat: ${labelStatus[st]}` : 'Belum ada aset'}
                />
                <span className="ph-klien__nama">{k.nama}</span>
                <span className="ph-klien__n num">{k.jumlah_aset} aset</span>
                <Icon name="chevron-right" />
              </Tautan>
            )
          })}
        </div>
        <div className="ph-sheet__foot">
          <button
            type="button"
            className="btn btn--secondary is-lg"
            onClick={() => {
              onClose()
              aksi.tambahKlien()
            }}
          >
            <Icon name="plus" />
            Tambah klien
          </button>
        </div>
      </div>
    </>
  )
}

function Ringkasan({ items, saring, setSaring, aksi }: Pick<LayarProps, 'items' | 'saring' | 'setSaring' | 'aksi'>) {
  // Only the status chips apply on the phone.
  const sp = { ...saringAwal, status: saring.status }
  const rows = saringItem(items, sp)
  const perStatus = tanpaSaring(items, sp, 'status')
  const j = judul(items)
  const b = biaya30(items)
  const urut = [...items].sort((a, c) => a.sisa_hari - c.sisa_hari)
  return (
    <main>
      <div className="ph__hero">
        <div className="eyebrow">{tanggalPanjang(hariIniIso())}</div>
        <h1 className="display">
          {j.utama}
          {j.sunyi ? <span className="quiet"> {j.sunyi}</span> : null}
        </h1>
        <p className="lede">
          {b.total > 0 ? (
            <>
              <b className="num">{rupiahBiasa(b.total)}</b> jatuh tempo dalam 30 hari ke depan.
            </>
          ) : (
            'Tidak ada biaya perpanjangan dalam 30 hari ke depan.'
          )}
        </p>
      </div>
      <div
        className="ph__bar"
        role="img"
        aria-label={urutanStatus.map((s) => `${items.filter((it) => it.status === s).length} ${labelStatus[s].toLowerCase()}`).join(', ')}
      >
        {urut.map((it) => (
          <span key={it.id} className={`st-${kelasStatus[it.status]}`} />
        ))}
      </div>
      <div className="ph__blok">
        <PerluDikerjakan />
      </div>
      <h2 className="blok__judul ph__judul">Semua aset</h2>
      <div className="ph__chips">
        <div className="chips" role="group" aria-label="Saring menurut status">
          <button type="button" className="chip" aria-pressed={saring.status === null} onClick={() => setSaring((s) => ({ ...s, status: null }))}>
            Semua <span className="num">{perStatus.length}</span>
          </button>
          {urutanStatus.map((st) => (
            <button
              key={st}
              type="button"
              className="chip"
              aria-pressed={saring.status === st}
              onClick={() => setSaring((s) => ({ ...s, status: s.status === st ? null : st }))}
            >
              <span className={`dot st-${kelasStatus[st]}`} />
              {labelStatus[st]} <span className="num">{perStatus.filter((it) => it.status === st).length}</span>
            </button>
          ))}
        </div>
      </div>
      <DaftarAsetPhone rows={rows} aksi={aksi} />
      {rows.length === 0 ? (
        <div className="ph-empty">
          <b>Tidak ada aset yang cocok.</b>
          <button type="button" className="btn btn--secondary is-lg" onClick={() => setSaring((s) => ({ ...s, status: null }))}>
            Tampilkan semua
          </button>
        </div>
      ) : null}
    </main>
  )
}

export function Phone({ items, klien, saring, setSaring, aksi, muat, akun, rute }: ShellProps) {
  const [sheet, setSheet] = useState(false)
  const siap = muat.status === 'siap'
  const kosong = siap && items.length === 0
  const k = rute.nama === 'klien' ? klien.find((x) => x.id === rute.id) : undefined

  let isi
  if (rute.nama === 'pengaturan') {
    isi = (
      <main>
        <Pengaturan akun={akun} phone />
      </main>
    )
  } else if (muat.status === 'memuat') {
    isi = (
      <div className="ph__hero" aria-busy="true" aria-label="Memuat data">
        <span className="sk" style={{ width: 180, height: 14 }} />
        <span className="sk" style={{ width: '85%', height: 32, marginTop: 12 }} />
        <span className="sk" style={{ width: '60%', height: 32, marginTop: 8 }} />
        <span className="sk" style={{ width: '100%', height: 8, marginTop: 24 }} />
        {Array.from({ length: 4 }, (_, i) => (
          <span key={i} className="sk" style={{ width: '100%', height: 56, marginTop: 16 }} />
        ))}
      </div>
    )
  } else if (muat.status === 'galat') {
    isi = <Galat muat={muat} phone />
  } else if (rute.nama === 'klien') {
    isi = k ? (
      <main>
        <RuangKlien klien={k} items={items} semuaKlien={klien} tab={rute.tab} saring={saring} setSaring={setSaring} aksi={aksi} phone />
      </main>
    ) : (
      <TidakAda judul="Klien tidak ditemukan." teks="Klien ini sudah dihapus atau alamatnya salah." phone />
    )
  } else if (kosong) {
    isi = <EmptyState aksi={aksi} phone />
  } else {
    isi = <Ringkasan items={items} saring={saring} setSaring={setSaring} aksi={aksi} />
  }

  return (
    <div className="ph">
      <header className="ph__head">
        <Tautan ke="/app" className="wordmark" aria-label="Tenggat, ke ringkasan">
          tenggat
        </Tautan>
        {rute.nama === 'ringkasan' && siap && !kosong ? (
          <div className="acts">
            <EksporCsv className="btn btn--secondary is-lg btn--icon" ikonSaja />
            <button type="button" className="btn btn--primary is-lg" onClick={() => aksi.tambah()}>
              <Icon name="plus" />
              Tambah aset
            </button>
          </div>
        ) : null}
      </header>

      {isi}

      <nav className="tabbar" aria-label="Menu">
        <Tautan ke="/app" className="tab" aria-current={rute.nama === 'ringkasan' ? 'page' : undefined}>
          <Icon name="layout-dashboard" />
          Ringkasan
        </Tautan>
        <button
          type="button"
          className={rute.nama === 'klien' ? 'tab is-active' : 'tab'}
          aria-haspopup="dialog"
          aria-expanded={sheet}
          disabled={!siap}
          onClick={() => setSheet(true)}
        >
          <Icon name="users" />
          Klien
        </button>
        <Tautan ke="/app/pengaturan" className="tab" aria-current={rute.nama === 'pengaturan' ? 'page' : undefined}>
          <Icon name="settings" />
          Pengaturan
        </Tautan>
      </nav>
      {sheet ? (
        <SheetKlien
          klien={klien}
          terburuk={statusTerburuk(items)}
          aktifId={rute.nama === 'klien' ? rute.id : null}
          aksi={aksi}
          onClose={() => setSheet(false)}
        />
      ) : null}
    </div>
  )
}
