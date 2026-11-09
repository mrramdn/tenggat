// Desktop asset list: type tabs, the client and source dropdowns, the rows and their menu. The overview shows every
// asset with a client dropdown; a client's space passes only that client's assets and hides the client column.
import { useCallback, useState } from 'react'
import type { Item, Jenis, Klien, Sumber } from '../lib/api'
import { adaSaring, saringAwal, saringItem, tanpaSaring, type Saring } from '../lib/derive'
import { frasaHari, labelJenis, lalu, rupiahBiasa, tanggal } from '../lib/format'
import { tautanPengingat } from '../lib/pengingat'
import { Icon, ikonJenis } from './Icon'
import { Pilihan } from './Pilihan'
import { RowMenu } from './RowMenu'
import type { Aksi } from './types'
import { Days, Pill } from './ui'

function Check({ it }: { it: Item }) {
  if (it.sumber === 'server' || it.sumber === 'registri') {
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
          <Icon name={it.sumber === 'registri' ? 'database' : 'server'} />
          {it.sumber === 'registri' ? 'Dari registri' : 'Dari server'}
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
  const detail =
    it.jenis === 'sertifikat'
      ? it.tls?.penerbit
      : it.sumber === 'registri' && it.registri?.registrar
        ? it.registri.registrar
        : it.biaya
          ? rupiahBiasa(it.biaya)
          : ''
  return detail ? `${labelJenis[it.jenis]} · ${detail}` : labelJenis[it.jenis]
}

function Row({ it, aksi, menuTerbuka, bukaMenu }: { it: Item; aksi: Aksi; menuTerbuka: boolean; bukaMenu: (el: HTMLElement) => void }) {
  // Certificates and registry domains are read again on request; the rest is extended by hand.
  const bisaCek = !!it.host && (it.jenis === 'sertifikat' || it.sumber === 'registri')
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
        {bisaCek ? (
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

interface Props {
  items: Item[]
  klien: Klien[]
  saring: Saring
  setSaring: (f: (s: Saring) => Saring) => void
  aksi: Aksi
  /** Inside a client's space: no client dropdown and no client column. */
  satuKlien?: boolean
}

export function DaftarAset({ items, klien, saring, setSaring, aksi, satuKlien = false }: Props) {
  // In a client's space the client filter of the overview does not apply.
  const s: Saring = satuKlien ? { ...saring, klien: null } : saring
  const rows = saringItem(items, s)
  const perJenis = tanpaSaring(items, s, 'jenis')
  const perKlien = tanpaSaring(items, s, 'klien')
  const perSumber = tanpaSaring(items, s, 'sumber')
  const jenis = items.some((it) => it.jenis === 'lainnya') ? [...JENIS_UTAMA, 'lainnya' as const] : JENIS_UTAMA
  const [menu, setMenu] = useState<{ it: Item; el: HTMLElement } | null>(null)
  const tutupMenu = useCallback((kembali: boolean) => {
    setMenu((m) => {
      if (m && kembali) m.el.focus()
      return null
    })
  }, [])

  return (
    <section className={satuKlien ? 'list is-klien' : 'list'} aria-label="Daftar aset">
      <div className="toolbar">
        <div className="seg" role="radiogroup" aria-label="Jenis aset">
          <button
            type="button"
            className="seg__item"
            role="radio"
            aria-checked={s.jenis === null}
            onClick={() => setSaring((x) => ({ ...x, jenis: null }))}
          >
            Semua <span className="seg__count num">{perJenis.length}</span>
          </button>
          {jenis.map((j) => (
            <button
              key={j}
              type="button"
              className="seg__item"
              role="radio"
              aria-checked={s.jenis === j}
              onClick={() => setSaring((x) => ({ ...x, jenis: j }))}
            >
              {labelJenis[j]} <span className="seg__count num">{perJenis.filter((it) => it.jenis === j).length}</span>
            </button>
          ))}
        </div>
        <div className="toolbar__group">
          {satuKlien ? null : (
            <Pilihan
              id="saring-klien"
              label="Klien"
              className="toolbar__pilih"
              nilai={s.klien ?? ''}
              onPilih={(v) => setSaring((x) => ({ ...x, klien: v || null }))}
              opsi={[
                { nilai: '', label: 'Semua klien', meta: perKlien.length },
                ...klien.map((k) => ({ nilai: k.id, label: k.nama, meta: perKlien.filter((it) => it.klien_id === k.id).length })),
              ]}
            />
          )}
          <Pilihan
            label="Sumber tanggal"
            className="toolbar__pilih"
            nilai={s.sumber ?? ''}
            onPilih={(v) => setSaring((x) => ({ ...x, sumber: (v || null) as Sumber | null }))}
            opsi={[
              { nilai: '', label: 'Semua sumber', meta: perSumber.length },
              { nilai: 'server', label: 'Dari server', meta: perSumber.filter((it) => it.sumber === 'server').length },
              { nilai: 'registri', label: 'Dari registri', meta: perSumber.filter((it) => it.sumber === 'registri').length },
              { nilai: 'manual', label: 'Manual', meta: perSumber.filter((it) => it.sumber === 'manual').length },
            ]}
          />
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
              className={s.naik ? 'sort' : 'sort is-desc'}
              aria-label={s.naik ? 'Sisa hari, terdekat dulu. Ubah urutan' : 'Sisa hari, terjauh dulu. Ubah urutan'}
              onClick={() => setSaring((x) => ({ ...x, naik: !x.naik }))}
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
            {items.length === 0 ? (
              <>
                <b>Klien ini belum punya aset.</b>
                <span>Tambahkan domain, sertifikat, token, atau langganan milik klien ini.</span>
              </>
            ) : (
              <>
                <b>Tidak ada aset yang cocok.</b>
                <span>Ubah pencarian atau saringan, atau tampilkan semua aset.</span>
              </>
            )}
            {adaSaring(s) && items.length > 0 ? (
              <button
                type="button"
                className="btn btn--secondary"
                onClick={() => setSaring((x) => ({ ...saringAwal, klien: satuKlien ? x.klien : null, naik: x.naik }))}
              >
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
          onPengingat={() => {
            setMenu(null)
            window.open(tautanPengingat(menu.it), '_blank', 'noopener,noreferrer')
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
