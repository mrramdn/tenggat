// Desktop dashboard: sidebar, header, hero with overview, filters and the list.
import { useCallback, useState } from 'react'
import { api, type Item, type Jenis, type Sumber } from '../lib/api'
import {
  adaSaring,
  daftarKlien,
  hitungStatus,
  judul,
  kalimatBiaya,
  rentangTanggal,
  saringAwal,
  saringItem,
  tanpaSaring,
} from '../lib/derive'
import {
  frasaHari,
  hariIniIso,
  labelJenis,
  lalu,
  rupiahBiasa,
  tanggal,
  tanggalPanjang,
  urutanStatus,
} from '../lib/format'
import { EmptyState } from './EmptyState'
import { Icon, ikonJenis } from './Icon'
import { RowMenu } from './RowMenu'
import { Timeline } from './Timeline'
import type { Aksi, LayarProps } from './types'
import { Days, Pill } from './ui'

export type Muat = { status: 'memuat' } | { status: 'galat'; pesan: string; cobaLagi: () => void } | { status: 'siap' }

export function fokusKlien() {
  const el = document.getElementById('saring-klien') as HTMLSelectElement | null
  if (!el) return
  el.focus()
  try {
    el.showPicker?.()
  } catch {
    /* some browsers refuse showPicker for select; focus is enough */
  }
}

function Sidebar({ items, saring, setSaring, muat }: LayarProps & { muat: Muat['status'] }) {
  const siap = muat === 'siap'
  const klien = daftarKlien(items)
  return (
    <aside className="side">
      <div className="wordmark">tenggat</div>
      <label className={saring.cari ? 'input-icon' : 'input-icon has-kbd'}>
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
        {saring.cari ? null : <span className="kbd" aria-hidden="true">/</span>}
      </label>
      <nav className="nav" aria-label="Menu">
        <button
          type="button"
          className="nav__item is-active"
          aria-current="page"
          title="Tampilkan semua aset"
          onClick={() => setSaring(saringAwal)}
        >
          <Icon name="layers" />
          Aset
          <span className="count num">{siap ? items.length : ''}</span>
        </button>
        <button type="button" className="nav__item" title="Saring menurut klien" onClick={fokusKlien} disabled={!siap || klien.length === 0}>
          <Icon name="users" />
          Klien
          <span className="count num">{siap ? klien.length : ''}</span>
        </button>
        <a className="nav__item" href={api.kalenderUrl} download="tenggat.ics" title="Unduh kalender (.ics)">
          <Icon name="calendar" />
          Kalender
        </a>
      </nav>
      <nav className="nav" aria-label="Klien" hidden={muat === 'galat'}>
        <div className="nav__label">Klien</div>
        {muat === 'memuat'
          ? [96, 120, 80].map((w) => (
              <div key={w} className="nav__item is-client is-note">
                <span className="sk" style={{ width: w, height: 12 }} />
              </div>
            ))
          : null}
        {siap && klien.length === 0 ? <div className="nav__item is-client is-note">Belum ada klien</div> : null}
        {klien.map((k) => {
          const aktif = saring.klien === k.nama
          return (
            <button
              key={k.nama}
              type="button"
              className={aktif ? 'nav__item is-client is-active' : 'nav__item is-client'}
              aria-pressed={aktif}
              onClick={() => setSaring((s) => ({ ...s, klien: aktif ? null : k.nama }))}
            >
              <span>{k.nama}</span>
              <span className="count num">{k.n}</span>
            </button>
          )
        })}
      </nav>
      <div className="side__foot">
        <div className="privacy">
          <Icon name="shield" />
          <span>Tenggat hanya menyimpan nama dan tanggal. Jangan simpan nilai token atau kata sandi.</span>
        </div>
      </div>
    </aside>
  )
}

function Hero({ items, saring, setSaring }: Omit<LayarProps, 'aksi'>) {
  const j = judul(items)
  const b = kalimatBiaya(items)
  const n = hitungStatus(items)
  return (
    <section className="hero">
      <div className="hero__text">
        <div className="eyebrow">
          {tanggalPanjang(hariIniIso())}
          <span className="sep" />
          <span className="num">{items.length} aset dipantau</span>
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

function Check({ it }: { it: Item }) {
  if (it.sumber === 'server') {
    if (it.galat_cek) {
      return (
        <div className="check is-fail">
          <span className="check__src">
            <Icon name="alert" />
            Cek gagal{it.terakhir_dicek ? ` · ${lalu(it.terakhir_dicek)}` : ''}
          </span>
          <span className="check__meta" title={it.galat_cek}>
            {it.galat_cek.replace(/\.$/, '')}
          </span>
        </div>
      )
    }
    return (
      <div className="check">
        <span className="check__src">
          <Icon name="server" />
          Dari server
        </span>
        <span className="check__meta">{it.terakhir_dicek ? `Dicek ${lalu(it.terakhir_dicek)}` : 'Belum dicek'}</span>
      </div>
    )
  }
  return (
    <div className="check">
      <span className="check__src">
        <Icon name="pencil" />
        Manual
      </span>
      <span className="check__meta">Tanggal diisi Anda</span>
    </div>
  )
}

export function subJenis(it: Item): string {
  const detail = it.jenis === 'sertifikat' ? it.tls?.penerbit : it.biaya ? rupiahBiasa(it.biaya) : ''
  return detail ? `${labelJenis[it.jenis]} · ${detail}` : labelJenis[it.jenis]
}

function Row({ it, aksi, menuTerbuka, bukaMenu }: { it: Item; aksi: Aksi; menuTerbuka: boolean; bukaMenu: (el: HTMLElement) => void }) {
  const sertifikat = it.jenis === 'sertifikat' && !!it.host
  const cek = aksi.sedangCek.has(it.id)
  return (
    <div className="row" role="listitem" aria-label={`${it.nama}, ${frasaHari(it.sisa_hari)}`}>
      <div className="sisa">
        <Days n={it.sisa_hari} />
        <Pill status={it.status} />
      </div>
      <div className="asset">
        <div className="asset__name" title={it.nama}>
          {it.nama}
        </div>
        <div className="asset__sub">
          <Icon name={ikonJenis[it.jenis]} />
          <span>
            {subJenis(it)}
            {it.klien ? <span className="sub-klien"> · {it.klien}</span> : null}
            <span className="sub-tgl"> · {tanggal(it.kedaluwarsa)}</span>
          </span>
        </div>
      </div>
      <div className={it.klien ? 'cell col-client' : 'cell col-client is-empty'}>{it.klien || 'Tanpa klien'}</div>
      <div className="cell num col-date">{tanggal(it.kedaluwarsa)}</div>
      <Check it={it} />
      <div className="actions">
        {sertifikat ? (
          <button
            type="button"
            className={cek ? 'btn btn--secondary is-sm btn--fixed is-busy' : 'btn btn--secondary is-sm btn--fixed'}
            aria-busy={cek}
            onClick={() => aksi.cekUlang(it)}
          >
            <Icon name={cek ? 'loader' : 'refresh'} className={cek ? 'spin' : undefined} />
            {cek ? 'Memeriksa' : 'Cek ulang'}
          </button>
        ) : (
          <button type="button" className="btn btn--secondary is-sm btn--fixed" onClick={() => aksi.ubah(it, 'kedaluwarsa')}>
            <Icon name="calendar" />
            Perpanjang
          </button>
        )}
        <button
          type="button"
          className="btn btn--ghost is-sm btn--icon"
          aria-label={`Tindakan lain untuk ${it.nama}`}
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

const JENIS_UTAMA: Jenis[] = ['domain', 'sertifikat', 'token', 'langganan']

function List({ items, saring, setSaring, aksi }: LayarProps) {
  const rows = saringItem(items, saring)
  const perJenis = tanpaSaring(items, saring, 'jenis')
  const jenis = items.some((it) => it.jenis === 'lainnya') ? [...JENIS_UTAMA, 'lainnya' as const] : JENIS_UTAMA
  const klien = daftarKlien(items)
  const [menu, setMenu] = useState<{ it: Item; el: HTMLElement } | null>(null)
  const tutupMenu = useCallback(
    (kembali: boolean) => {
      setMenu((m) => {
        if (m && kembali) m.el.focus()
        return null
      })
    },
    [],
  )

  return (
    <section className="list" aria-label="Daftar aset">
      <div className="toolbar">
        <div className="seg" role="radiogroup" aria-label="Jenis aset">
          <button
            type="button"
            className="seg__item"
            role="radio"
            aria-checked={saring.jenis === null}
            onClick={() => setSaring((s) => ({ ...s, jenis: null }))}
          >
            Semua <span className="seg__count num">{perJenis.length}</span>
          </button>
          {jenis.map((j) => (
            <button
              key={j}
              type="button"
              className="seg__item"
              role="radio"
              aria-checked={saring.jenis === j}
              onClick={() => setSaring((s) => ({ ...s, jenis: j }))}
            >
              {labelJenis[j]} <span className="seg__count num">{perJenis.filter((it) => it.jenis === j).length}</span>
            </button>
          ))}
        </div>
        <div className="toolbar__group">
          <select
            id="saring-klien"
            className="control"
            aria-label="Klien"
            value={saring.klien ?? ''}
            onChange={(e) => setSaring((s) => ({ ...s, klien: e.target.value || null }))}
          >
            <option value="">Semua klien</option>
            {klien.map((k) => (
              <option key={k.nama} value={k.nama}>
                {k.nama}
              </option>
            ))}
          </select>
          <select
            className="control"
            aria-label="Sumber tanggal"
            value={saring.sumber ?? ''}
            onChange={(e) => setSaring((s) => ({ ...s, sumber: (e.target.value || null) as Sumber | null }))}
          >
            <option value="">Semua sumber</option>
            <option value="server">Dari server</option>
            <option value="manual">Manual</option>
          </select>
          <div className="toolbar__end num" aria-live="polite">
            {rows.length} dari {items.length} aset
          </div>
        </div>
      </div>
      <div className="rows">
        <div className="row row--head">
          <span className="sisa">
            <button
              type="button"
              className={saring.naik ? 'sort' : 'sort is-desc'}
              aria-label={saring.naik ? 'Sisa hari, terdekat dulu. Ubah urutan' : 'Sisa hari, terjauh dulu. Ubah urutan'}
              onClick={() => setSaring((s) => ({ ...s, naik: !s.naik }))}
            >
              Sisa
              <Icon name="arrow-down" />
            </button>
            <span>Status</span>
          </span>
          <span>Aset</span>
          <span className="col-client">Klien</span>
          <span className="col-date">Berakhir</span>
          <span>Pemeriksaan</span>
          <span />
        </div>
        <div role="list">
          {rows.map((it) => (
            <Row
              key={it.id}
              it={it}
              aksi={aksi}
              menuTerbuka={menu?.it.id === it.id}
              bukaMenu={(el) => setMenu((m) => (m?.it.id === it.id ? null : { it, el }))}
            />
          ))}
        </div>
        {rows.length === 0 ? (
          <div className="rows__empty">
            <b>Tidak ada aset yang cocok.</b>
            <span>Ubah pencarian atau saringan, atau tampilkan semua aset.</span>
            {adaSaring(saring) ? (
              <button type="button" className="btn btn--secondary" onClick={() => setSaring((s) => ({ ...saringAwal, naik: s.naik }))}>
                Tampilkan semua
              </button>
            ) : null}
          </div>
        ) : null}
      </div>
      {menu ? (
        <RowMenu
          anchor={menu.el}
          onClose={tutupMenu}
          onUbah={() => {
            setMenu(null)
            aksi.ubah(menu.it)
          }}
          onHapus={() => {
            setMenu(null)
            aksi.hapus(menu.it)
          }}
        />
      ) : null}
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

export function Desktop(props: LayarProps & { muat: Muat }) {
  const { items, saring, aksi, muat } = props
  const siap = muat.status === 'siap'
  const kosong = siap && items.length === 0
  return (
    <div className="app">
      <Sidebar {...props} muat={muat.status} />
      <main className="main">
        <header className="topbar">
          <div className="crumbs">
            <b>Aset</b>
            {kosong ? null : (
              <>
                <span>/</span>
                <span>{saring.klien ?? 'Semua klien'}</span>
              </>
            )}
          </div>
          {kosong ? null : (
            <div className="topbar__end">
              <a className="btn btn--secondary" href={api.kalenderUrl} download="tenggat.ics">
                <Icon name="download" />
                Unduh kalender (.ics)
              </a>
              <button type="button" className="btn btn--primary" onClick={() => aksi.tambah()} aria-keyshortcuts="N">
                <Icon name="plus" />
                Tambah aset
              </button>
            </div>
          )}
        </header>
        {muat.status === 'memuat' ? <Skeleton /> : null}
        {muat.status === 'galat' ? (
          <div className="state" role="alert">
            <h1 className="display">Data tidak dapat dimuat.</h1>
            <p>{muat.pesan}</p>
            <button type="button" className="btn btn--primary" onClick={muat.cobaLagi}>
              <Icon name="refresh" />
              Coba lagi
            </button>
          </div>
        ) : null}
        {kosong ? <EmptyState aksi={aksi} /> : null}
        {siap && !kosong ? (
          <div className="content">
            <Hero {...props} />
            <List {...props} />
          </div>
        ) : null}
      </main>
    </div>
  )
}

