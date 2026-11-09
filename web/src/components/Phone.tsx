// Phone layout: top bar, headline, status bar and chips, grouped list, tab bar.
import { api, type Item, type Status } from '../lib/api'
import { biaya30, daftarKlien, judul, saringAwal, saringItem, tanpaSaring } from '../lib/derive'
import { hariIniIso, kelasStatus, labelJenisPendek, labelStatus, lalu, rupiahBiasa, tanggalPanjang, tanggalPendek, urutanStatus } from '../lib/format'
import { AkunTab } from './Akun'
import type { Muat } from './Desktop'
import { EmptyState } from './EmptyState'
import { Icon, ikonJenis } from './Icon'
import type { Aksi, LayarProps } from './types'
import { Days, Pill } from './ui'

function PhoneRow({ it, aksi }: { it: Item; aksi: Aksi }) {
  const cek = aksi.sedangCek.has(it.id)
  return (
    <div className="ph-row">
      <Days n={it.sisa_hari} />
      <div className="asset">
        <button type="button" className="asset__name" onClick={() => aksi.ubah(it)} aria-label={`${it.nama}. Buka untuk mengubah`}>
          {it.nama}
        </button>
        <div className="asset__sub">
          <Icon name={ikonJenis[it.jenis]} />
          <span>
            {labelJenisPendek[it.jenis]}
            {it.klien ? ` · ${it.klien}` : ''}
          </span>
        </div>
      </div>
      <div className="ph-row__end">
        <Pill status={it.status} />
        <span className="cell num">{tanggalPendek(it.kedaluwarsa)}</span>
      </div>
      {it.galat_cek ? (
        <>
          <div className="ph-row__fail">
            <Icon name="alert" />
            <div>
              Cek gagal{it.terakhir_dicek ? ` ${lalu(it.terakhir_dicek)}` : ''}. <span>{it.galat_cek}</span>
            </div>
          </div>
          <div className="ph-row__act">
            <button type="button" className={cek ? 'btn btn--secondary is-lg is-busy' : 'btn btn--secondary is-lg'} aria-busy={cek} onClick={() => aksi.cekUlang(it)}>
              <Icon name={cek ? 'loader' : 'refresh'} className={cek ? 'spin' : undefined} />
              {cek ? 'Memeriksa' : 'Cek ulang'}
            </button>
          </div>
        </>
      ) : null}
    </div>
  )
}

const GRUP: { judul: string; status: Status[] }[] = [
  { judul: 'Perlu tindakan', status: ['lewat', 'kritis'] },
  { judul: '30 hari ke depan', status: ['segera'] },
  { judul: 'Aman', status: ['aman'] },
]

export function Phone({ items, saring, setSaring, aksi, muat, akun }: LayarProps & { muat: Muat }) {
  const siap = muat.status === 'siap'
  const kosong = siap && items.length === 0
  // Only the filters the phone shows apply here.
  const sp = { ...saringAwal, status: saring.status, klien: saring.klien }
  const rows = saringItem(items, sp)
  const perStatus = tanpaSaring(items, sp, 'status')
  const j = judul(items)
  const b = biaya30(items)
  const klien = daftarKlien(items)
  const urut = [...items].sort((a, c) => a.sisa_hari - c.sisa_hari)

  return (
    <div className="ph">
      <header className="ph__head">
        <div className="wordmark">tenggat</div>
        <div className="acts">
          <a className="btn btn--secondary is-lg btn--icon" href={api.kalenderUrl} download="tenggat.ics" aria-label="Unduh kalender (.ics)">
            <Icon name="download" />
          </a>
          <button type="button" className="btn btn--primary is-lg" onClick={() => aksi.tambah()}>
            <Icon name="plus" />
            Tambah aset
          </button>
        </div>
      </header>

      {muat.status === 'memuat' ? (
        <div className="ph__hero" aria-busy="true" aria-label="Memuat data">
          <span className="sk" style={{ width: 180, height: 14 }} />
          <span className="sk" style={{ width: '85%', height: 32, marginTop: 12 }} />
          <span className="sk" style={{ width: '60%', height: 32, marginTop: 8 }} />
          <span className="sk" style={{ width: '100%', height: 8, marginTop: 24 }} />
          {Array.from({ length: 4 }, (_, i) => (
            <span key={i} className="sk" style={{ width: '100%', height: 56, marginTop: 16 }} />
          ))}
        </div>
      ) : null}

      {muat.status === 'galat' ? (
        <div className="state" role="alert">
          <h1 className="display">Data tidak dapat dimuat.</h1>
          <p>{muat.pesan}</p>
          <button type="button" className="btn btn--primary is-lg" onClick={muat.cobaLagi}>
            <Icon name="refresh" />
            Coba lagi
          </button>
        </div>
      ) : null}

      {kosong ? <EmptyState aksi={aksi} phone /> : null}

      {siap && !kosong ? (
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
                'Tidak ada biaya jatuh tempo dalam 30 hari ke depan.'
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
          {saring.klien !== null ? (
            <div className="ph-filter">
              <span>
                Klien: <b>{saring.klien}</b>
              </span>
              <button type="button" className="btn btn--ghost is-lg" onClick={() => setSaring((s) => ({ ...s, klien: null }))}>
                Semua klien
              </button>
            </div>
          ) : null}
          {GRUP.map((g) => {
            const isi = rows.filter((it) => g.status.includes(it.status))
            if (isi.length === 0) return null
            return (
              <section key={g.judul} aria-label={g.judul}>
                <div className="ph-group">
                  <span>{g.judul}</span>
                  <span className="num">{isi.length}</span>
                </div>
                {isi.map((it) => (
                  <PhoneRow key={it.id} it={it} aksi={aksi} />
                ))}
              </section>
            )
          })}
          {rows.length === 0 ? (
            <div className="ph-empty">
              <b>Tidak ada aset yang cocok.</b>
              <button type="button" className="btn btn--secondary is-lg" onClick={() => setSaring(saringAwal)}>
                Tampilkan semua
              </button>
            </div>
          ) : null}
        </main>
      ) : null}

      <nav className="tabbar" aria-label="Menu">
        <button
          type="button"
          className="tab is-active"
          aria-current="page"
          onClick={() => {
            setSaring(saringAwal)
            window.scrollTo({ top: 0 })
          }}
        >
          <Icon name="layers" />
          Aset
        </button>
        <label className={saring.klien !== null ? 'tab is-active' : 'tab'}>
          <Icon name="users" />
          Klien
          <select
            aria-label="Saring menurut klien"
            value={saring.klien ?? ''}
            disabled={klien.length === 0}
            onChange={(e) => {
              setSaring((s) => ({ ...s, klien: e.target.value || null }))
              window.scrollTo({ top: 0 })
            }}
          >
            <option value="">Semua klien</option>
            {klien.map((k) => (
              <option key={k.nama} value={k.nama}>
                {k.nama} ({k.n})
              </option>
            ))}
          </select>
        </label>
        <a className="tab" href={api.kalenderUrl} download="tenggat.ics">
          <Icon name="calendar" />
          Kalender
        </a>
        <AkunTab akun={akun} />
      </nav>
    </div>
  )
}
