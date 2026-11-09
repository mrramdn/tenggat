// Desktop shell: the sidebar (Ringkasan, the clients, Pengaturan, the account) and the main panel, whose content
// follows the route: the overview, a client's space or the settings.
import type { ReactNode } from 'react'
import { hitungStatus, judul, kalimatBiaya, rentangTanggal, statusTerburuk } from '../lib/derive'
import { hariIniIso, kelasStatus, labelStatus, tanggalPanjang, urutanStatus } from '../lib/format'
import { jalurKlien, Tautan, type Rute } from '../lib/rute'
import { AkunSidebar } from './Akun'
import { DaftarAset } from './DaftarAset'
import { EksporCsv } from './EksporCsv'
import { EmptyState } from './EmptyState'
import { Icon } from './Icon'
import { Pengaturan } from './Pengaturan'
import { PerluDikerjakan } from './PerluDikerjakan'
import { RuangKlien } from './RuangKlien'
import { Timeline } from './Timeline'
import type { LayarProps } from './types'
import { Pill } from './ui'

export type Muat = { status: 'memuat' } | { status: 'galat'; pesan: string; cobaLagi: () => void } | { status: 'siap' }

export type ShellProps = LayarProps & { muat: Muat; rute: Rute }

function Sidebar({ items, klien, rute, aksi, muat, akun }: ShellProps) {
  const siap = muat.status === 'siap'
  const terburuk = statusTerburuk(items)
  return (
    <aside className="side">
      <Tautan ke="/app" className="wordmark" aria-label="Tenggat, ke ringkasan">
        tenggat
      </Tautan>
      <nav className="nav" aria-label="Menu">
        <Tautan ke="/app" className="nav__item" aria-current={rute.nama === 'ringkasan' ? 'page' : undefined}>
          <Icon name="layout-dashboard" />
          Ringkasan
          <span className="count num">{siap ? items.length : ''}</span>
        </Tautan>
      </nav>
      <nav className="nav nav--klien" aria-label="Klien" hidden={muat.status === 'galat'}>
        <div className="nav__label">Klien</div>
        {muat.status === 'memuat'
          ? [96, 120, 80].map((w) => (
              <div key={w} className="nav__item is-client is-note">
                <span className="sk" style={{ width: w, height: 12 }} />
              </div>
            ))
          : null}
        {siap && klien.length === 0 ? (
          <p className="side-note">Setiap klien punya halaman sendiri berisi aset, tugas, aktivitas, dan laporan bulanannya.</p>
        ) : null}
        {klien.map((k) => {
          const st = terburuk.get(k.id)
          const aktif = rute.nama === 'klien' && rute.id === k.id
          const pasif = k.status === 'jeda' || k.status === 'selesai'
          return (
            <Tautan
              key={k.id}
              ke={jalurKlien(k.id)}
              className={pasif ? 'nav__item is-client is-pasif' : 'nav__item is-client'}
              aria-current={aktif ? 'page' : undefined}
              title={k.nama}
            >
              <span
                className={st ? `nav__dot st-${kelasStatus[st]}` : 'nav__dot is-kosong'}
                role="img"
                aria-label={st ? `Status terdekat: ${labelStatus[st]}` : 'Belum ada aset'}
              />
              <span className="nav__nama">{k.nama}</span>
              <span className="count num" aria-label={`${k.jumlah_aset} aset`}>
                {k.jumlah_aset}
              </span>
            </Tautan>
          )
        })}
        {siap ? (
          <button type="button" className="nav__item is-client is-add" onClick={aksi.tambahKlien}>
            <Icon name="plus" />
            <span>Tambah klien</span>
          </button>
        ) : null}
      </nav>
      <div className="side__foot">
        <Tautan ke="/app/pengaturan" className="nav__item" aria-current={rute.nama === 'pengaturan' ? 'page' : undefined}>
          <Icon name="settings" />
          Pengaturan
        </Tautan>
        <AkunSidebar akun={akun} />
      </div>
    </aside>
  )
}

function Hero({ items, saring, setSaring, jumlahKlien }: Pick<LayarProps, 'items' | 'saring' | 'setSaring'> & { jumlahKlien: number }) {
  const j = judul(items)
  const b = kalimatBiaya(items)
  const n = hitungStatus(items)
  return (
    <section className="hero">
      <div className="hero__text">
        <div className="eyebrow">
          {tanggalPanjang(hariIniIso())}
          <span className="sep" />
          <span className="num">{items.length} aset</span>
          {jumlahKlien > 0 ? (
            <>
              <span className="sep" />
              <span className="num">{jumlahKlien} klien</span>
            </>
          ) : null}
        </div>
        <h1 className="display">
          {j.utama}
          {j.sunyi ? <span className="quiet"> {j.sunyi}</span> : null}
        </h1>
        <p className="lede">
          {b.pre}
          {b.jumlah ? <b className="num">{b.jumlah}</b> : null}
          {b.post}
        </p>
      </div>
      <div className="overview">
        <div className="stats" role="group" aria-label="Saring menurut status">
          {urutanStatus.map((st) => {
            const aktif = saring.status === st
            return (
              <button
                key={st}
                type="button"
                className="stat"
                aria-pressed={aktif}
                title={aktif ? 'Tampilkan semua status' : 'Saring daftar menurut status ini'}
                onClick={() => setSaring((s) => ({ ...s, status: aktif ? null : st }))}
              >
                <span className="stat__row">
                  <Pill status={st} />
                </span>
                <span className="stat__row">
                  <span className="stat__n num">{n[st]}</span>
                  <span className="stat__note num">{rentangTanggal(items, st)}</span>
                </span>
              </button>
            )
          })}
        </div>
        <div className="tl">
          <Timeline items={items} />
        </div>
      </div>
    </section>
  )
}

function Skeleton() {
  return (
    <div className="content" aria-busy="true" aria-label="Memuat data">
      <section className="hero">
        <div className="hero__text">
          <span className="sk" style={{ width: 220, height: 16 }} />
          <span className="sk" style={{ width: '80%', height: 44, marginTop: 16 }} />
          <span className="sk" style={{ width: '60%', height: 44, marginTop: 8 }} />
          <span className="sk" style={{ width: '90%', height: 14, marginTop: 'auto' }} />
        </div>
        <div className="overview" style={{ height: 242 }}>
          <div className="stats" style={{ height: 94 }}>
            {urutanStatus.map((s) => (
              <div key={s} className="stat" style={{ cursor: 'default' }}>
                <span className="sk" style={{ width: 56, height: 24 }} />
                <span className="sk" style={{ width: 32, height: 32 }} />
              </div>
            ))}
          </div>
          <div className="tl" style={{ gap: 12, paddingTop: 20 }}>
            {[0, 1, 2, 3].map((i) => (
              <span key={i} className="sk" style={{ width: '100%', height: 8 }} />
            ))}
          </div>
        </div>
      </section>
      <section className="list">
        <div className="toolbar">
          <span className="sk" style={{ width: 560, height: 36 }} />
          <span className="sk" style={{ width: 176, height: 36 }} />
          <span className="sk" style={{ width: 176, height: 36 }} />
        </div>
        <div className="rows">
          {Array.from({ length: 6 }, (_, i) => (
            <div key={i} className="row">
              <span className="sk" style={{ width: 100, height: 24 }} />
              <span className="sk" style={{ width: '70%', height: 16 }} />
              <span className="sk col-client" style={{ width: 96, height: 14 }} />
              <span className="sk col-date" style={{ width: 80, height: 14 }} />
              <span className="sk" style={{ width: 140, height: 14 }} />
              <span className="sk" style={{ width: 112, height: 32, justifySelf: 'end' }} />
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

/** The search field of the topbar; "/" focuses it. It filters the asset list on the page. */
function Cari({ saring, setSaring }: Pick<LayarProps, 'saring' | 'setSaring'>) {
  return (
    <label className={saring.cari ? 'input-icon topbar__cari' : 'input-icon has-kbd topbar__cari'}>
      <Icon name="search" />
      <input
        id="cari"
        type="search"
        className="control"
        placeholder="Cari aset"
        aria-label="Cari aset (tekan / untuk fokus)"
        autoComplete="off"
        value={saring.cari}
        onChange={(e) => setSaring((s) => ({ ...s, cari: e.target.value }))}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            setSaring((s) => ({ ...s, cari: '' }))
            e.currentTarget.blur()
          }
        }}
      />
      {saring.cari ? null : (
        <span className="kbd" aria-hidden="true">
          /
        </span>
      )}
    </label>
  )
}

/** A page inside the app that does not exist, such as an unknown client id. */
export function TidakAda({ judul: j, teks, phone = false }: { judul: string; teks: string; phone?: boolean }) {
  return (
    <div className="state" role="alert">
      <h1 className="display">{j}</h1>
      <p>{teks}</p>
      <Tautan ke="/app" className={phone ? 'btn btn--secondary is-lg' : 'btn btn--secondary'}>
        <Icon name="arrow-left" />
        Ke ringkasan
      </Tautan>
    </div>
  )
}

export function Galat({ muat, phone = false }: { muat: Extract<Muat, { status: 'galat' }>; phone?: boolean }) {
  return (
    <div className="state" role="alert">
      <h1 className="display">Data tidak dapat dimuat.</h1>
      <p>{muat.pesan}</p>
      <button type="button" className={phone ? 'btn btn--primary is-lg' : 'btn btn--primary'} onClick={muat.cobaLagi}>
        <Icon name="refresh" />
        Coba lagi
      </button>
    </div>
  )
}

export function Desktop(props: ShellProps) {
  const { items, klien, saring, setSaring, aksi, muat, rute, akun } = props
  const siap = muat.status === 'siap'
  const kosong = siap && items.length === 0
  const k = rute.nama === 'klien' ? klien.find((x) => x.id === rute.id) : undefined
  // Remount the scroll area per page, so a new page starts at the top.
  const kunciIsi = rute.nama === 'klien' ? `klien-${rute.id}-${rute.tab}` : rute.nama

  let crumbs: ReactNode
  let ujung: ReactNode = null
  let isi: ReactNode

  if (rute.nama === 'pengaturan') {
    crumbs = <b>Pengaturan</b>
    isi = (
      <div className="content" key={kunciIsi}>
        <Pengaturan akun={akun} />
      </div>
    )
  } else if (rute.nama === 'klien') {
    crumbs = k ? (
      <>
        <span>Klien</span>
        <span>/</span>
        <b>{k.nama}</b>
      </>
    ) : (
      <b>Klien</b>
    )
    if (k && rute.tab === 'aset' && items.some((it) => it.klien_id === k.id)) ujung = <Cari saring={saring} setSaring={setSaring} />
    isi =
      muat.status === 'memuat' ? (
        <div className="content" aria-busy="true" aria-label="Memuat data">
          <span className="sk" style={{ width: 280, height: 32 }} />
          <span className="sk" style={{ width: 420, height: 14 }} />
          <span className="sk" style={{ width: '100%', height: 240 }} />
        </div>
      ) : muat.status === 'galat' ? (
        <Galat muat={muat} />
      ) : k ? (
        <div className="content" key={kunciIsi}>
          <RuangKlien klien={k} items={items} semuaKlien={klien} tab={rute.tab} saring={saring} setSaring={setSaring} aksi={aksi} />
        </div>
      ) : (
        <TidakAda judul="Klien tidak ditemukan." teks="Klien ini sudah dihapus atau alamatnya salah." />
      )
  } else {
    crumbs = <b>Ringkasan</b>
    if (!kosong && siap) {
      ujung = (
        <>
          <Cari saring={saring} setSaring={setSaring} />
          <EksporCsv className="btn btn--secondary" />
          <button type="button" className="btn btn--primary" onClick={() => aksi.tambah()} aria-keyshortcuts="N">
            <Icon name="plus" />
            Tambah aset
          </button>
        </>
      )
    }
    isi =
      muat.status === 'memuat' ? (
        <Skeleton />
      ) : muat.status === 'galat' ? (
        <Galat muat={muat} />
      ) : kosong ? (
        <EmptyState aksi={aksi} />
      ) : (
        <div className="content" key={kunciIsi}>
          <Hero items={items} saring={saring} setSaring={setSaring} jumlahKlien={klien.length} />
          <PerluDikerjakan />
          <section className="blok" aria-labelledby="judul-semua-aset">
            <h2 className="blok__judul" id="judul-semua-aset">
              Semua aset
            </h2>
            <DaftarAset items={items} klien={klien} saring={saring} setSaring={setSaring} aksi={aksi} />
          </section>
        </div>
      )
  }

  return (
    <div className="app">
      <Sidebar {...props} />
      <main className="main">
        <header className="topbar">
          <div className="crumbs">{crumbs}</div>
          {ujung ? <div className="topbar__end">{ujung}</div> : null}
        </header>
        {isi}
      </main>
    </div>
  )
}
