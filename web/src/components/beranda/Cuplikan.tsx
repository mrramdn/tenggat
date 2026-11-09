// Pieces of the app on the front page, drawn with the app's own pill, day count and controls from fictional data.
// Decorative: hidden from assistive tech and never focusable. Dates are relative to today so they never go stale.
import type { Status } from '../../lib/api'
import { angka, frasaHari, hariIniIso, keTanggal, kelasStatus, statusDariSisa, tanggal, tanggalPendek } from '../../lib/format'
import { bagianDurasi, durasi } from '../../lib/api-laporan'
import { Icon, type IconName } from '../Icon'
import { IkonLaporan } from '../IkonLaporan'
import { Days, Pill } from '../ui'

function hariKe(n: number): string {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  d.setDate(d.getDate() + n)
  return hariIniIso(d)
}

/* ---------- a client's space ---------- */

const KLIEN = [
  { nama: 'Toko Kue Mawar', n: 4, aktif: true },
  { nama: 'Kopi Senja', n: 3 },
  { nama: 'Studio Arsitek Rana', n: 6 },
  { nama: 'Klinik Gigi Sehati', n: 2 },
]

const TUGAS = [
  { judul: 'Perpanjang domain', sub: 'Tiap tahun · Rp 285.000', sisa: 4 },
  { judul: 'Update plugin WordPress', sub: 'Tiap bulan', sisa: 2 },
  { judul: 'Backup database', sub: 'Tiap minggu', sisa: 5 },
  { judul: 'Cek formulir pemesanan', sub: 'Tiap bulan', sisa: 19 },
]

const AKTIVITAS: { ikon: IconName; teks: string; meta: string; hari: number }[] = [
  { ikon: 'tick', teks: 'Selesai: Backup database', meta: '20 menit', hari: -2 },
  { ikon: 'pencil', teks: 'Mengganti foto banner promo Natal', meta: '35 menit', hari: -3 },
  { ikon: 'refresh', teks: 'Sertifikat tokokuemawar.id diperbarui sampai ' + tanggal(hariKe(87)), meta: 'otomatis', hari: -5 },
]

export function CuplikanRuang() {
  const tugas = [...TUGAS].sort((a, b) => a.sisa - b.sisa)
  return (
    <div className="frag frag--app" aria-hidden="true" inert>
      <aside className="fa__side">
        <div className="wordmark fa__wordmark">tenggat</div>
        <div className="fa__nav">
          <span className="fa__nav-item">
            <Icon name="layers" />
            Ringkasan
          </span>
        </div>
        <div className="fa__nav">
          <span className="fa__nav-label">Klien</span>
          {KLIEN.map((k) => (
            <span key={k.nama} className={k.aktif ? 'fa__nav-item is-client is-active' : 'fa__nav-item is-client'}>
              <span>{k.nama}</span>
              <span className="fa__count num">{k.n}</span>
            </span>
          ))}
        </div>
      </aside>
      <div className="fa__main">
        <div className="fa__top">
          <span className="crumbs">
            <span>Klien</span>
            <span>/</span>
            <b>Toko Kue Mawar</b>
          </span>
        </div>
        <div className="fa__body">
          <div className="fa__head">
            <div className="fa__nama">Toko Kue Mawar</div>
            <ul className="fa__fakta">
              <li>
                <b className="num">4</b> aset
              </li>
              <li>
                <b className="num">Rp 285.000</b> jatuh tempo dalam 30 hari
              </li>
            </ul>
          </div>
          <div className="fa__tabs">
            <span>Aset</span>
            <span className="is-active">Tugas</span>
            <span>Aktivitas</span>
            <span>Laporan</span>
          </div>
          <div className="fa__rows">
            {tugas.map((t) => (
              <div key={t.judul} className="fa__row">
                <div className="sisa">
                  <Days n={t.sisa} />
                  <Pill status={statusDariSisa(t.sisa)} />
                </div>
                <div className="asset">
                  <div className="asset__name">{t.judul}</div>
                  <div className="asset__sub">
                    <Icon name="repeat" />
                    <span>{t.sub}</span>
                  </div>
                </div>
                <span className="btn btn--secondary is-sm fa__selesai">
                  <Icon name="tick" />
                  Tandai selesai
                </span>
              </div>
            ))}
          </div>
          <div className="fa__feed">
            <div className="fa__feed-label">Aktivitas terbaru</div>
            {AKTIVITAS.map((a) => (
              <div key={a.teks} className="fa__feed-row">
                <Icon name={a.ikon} />
                <span className="fa__feed-teks">{a.teks}</span>
                <span className="fa__feed-meta num">
                  {tanggalPendek(hariKe(a.hari))} · {a.meta}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

/* ---------- the monthly report ---------- */

const fmtBulanPanjang = new Intl.DateTimeFormat('id-ID', { month: 'long', year: 'numeric' })

function bulanLalu() {
  const kini = new Date()
  const awal = new Date(kini.getFullYear(), kini.getMonth() - 1, 1)
  const iso = (hari: number) => hariIniIso(new Date(awal.getFullYear(), awal.getMonth(), hari))
  return { label: fmtBulanPanjang.format(awal), iso }
}

const PEKERJAAN = [
  { hari: 3, teks: 'Update WordPress ke 6.8 dan 14 plugin', menit: 45 },
  { hari: 14, teks: 'Mengganti plugin formulir yang tidak lagi diperbarui', menit: 90 },
  { hari: 21, teks: 'Mengganti foto banner promo atas permintaan klien', menit: 35 },
  { hari: 28, teks: 'Cek formulir pemesanan dan email masuk', menit: 15 },
]

const JATUH_TEMPO = [
  { nama: 'Domain tokokuemawar.id', sisa: 4, biaya: 285000 },
  { nama: 'Hosting 2 GB', sisa: 41, biaya: 189000 },
  { nama: 'Sertifikat tokokuemawar.id', sisa: 87, biaya: null },
]


export function CuplikanLaporan() {
  const b = bulanLalu()
  const total = PEKERJAAN.reduce((s, p) => s + p.menit, 0)
  return (
    <div className="frag frag--laporan" aria-hidden="true" inert>
      <div className="fl__bar">
        <span className="fl__bar-judul">Laporan {b.label}</span>
        <span className="fl__bar-aksi">
          <span className="btn btn--secondary is-sm">
            <Icon name="link" />
            Bagikan tautan
          </span>
          <span className="btn btn--secondary is-sm fl__opsional">
            <IkonLaporan name="message" />
            Kirim lewat WhatsApp
          </span>
          <span className="btn btn--primary is-sm">
            <IkonLaporan name="printer" />
            Cetak / Simpan PDF
          </span>
        </span>
      </div>
      <div className="fl__kertas">
        <div className="fl__kop">
          <div>
            <div className="fl__kecil">Laporan bulanan</div>
            <div className="fl__judul">Toko Kue Mawar</div>
          </div>
          <div className="fl__kop-kanan">
            <div className="fl__bulan">{b.label}</div>
            <div className="fl__kecil">Disusun oleh Nadia Putri</div>
          </div>
        </div>
        <div className="fl__angka">
          <div>
            <span className="fl__n num">{PEKERJAAN.length}</span>
            <span className="fl__u">pekerjaan</span>
          </div>
          <div>
            <span className="fl__n num is-durasi">
              {bagianDurasi(total).map(([n, s], i) => (
                <span key={s}>
                  {i > 0 ? ' ' : ''}
                  {n}
                  <span className="fl__satuan"> {s}</span>
                </span>
              ))}
            </span>
            <span className="fl__u">waktu kerja</span>
          </div>
          <div>
            <span className="fl__n num">1</span>
            <span className="fl__u">perpanjangan</span>
          </div>
        </div>
        <p className="fl__catatan">
          Plugin formulir lama sudah dua tahun tidak diperbarui pembuatnya, jadi diganti dengan yang masih dirawat. Domain
          perlu diperpanjang sebelum {tanggal(hariKe(4))}.
        </p>
        <div className="fl__bagian">Pekerjaan</div>
        <div className="fl__tabel">
          {PEKERJAAN.map((p, i) => (
            <div key={i} className="fl__baris">
              <span className="fl__tgl num">{tanggalPendek(b.iso(p.hari))}</span>
              <span className="fl__teks">{p.teks}</span>
              <span className="fl__dur num">{durasi(p.menit)}</span>
            </div>
          ))}
        </div>
        {/* A month that has ended lists the assets as they are today, as the real report does. */}
        <div className="fl__bagian">Daftar aset per {tanggal(hariKe(0))}</div>
        <div className="fl__tabel">
          {JATUH_TEMPO.map((j) => (
            <div key={j.nama} className="fl__baris is-tempo">
              <span className="fl__teks">{j.nama}</span>
              <span className="fl__tgl num">{tanggal(hariKe(j.sisa))}</span>
              <Pill status={statusDariSisa(j.sisa)} />
              <span className="fl__dur num">{j.biaya ? `Rp ${angka(j.biaya)}` : '-'}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

/* ---------- the calendar subscription ---------- */

const ACARA: { teks: string; sisa: number }[] = [
  { teks: 'Lisensi font Rana', sisa: -3 },
  { teks: 'Update plugin Mawar', sisa: 2 },
  { teks: 'Domain Mawar', sisa: 4 },
  { teks: 'Token Cloudflare', sisa: 9 },
  { teks: 'TLS Kopi Senja', sisa: 13 },
  { teks: 'VPS 2 vCPU', sisa: 22 },
  { teks: 'Workspace Rana', sisa: 27 },
]

const HARI = ['Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab', 'Min']
const fmtBulan = new Intl.DateTimeFormat('id-ID', { month: 'long' })
const fmtTahun = new Intl.DateTimeFormat('id-ID', { year: 'numeric' })

/** Five weeks from the Monday of last week, so every date below is on the grid whatever today is. */
export function CuplikanKalender() {
  const kini = keTanggal(hariIniIso())
  const mulai = new Date(kini)
  mulai.setDate(kini.getDate() - ((kini.getDay() + 6) % 7) - 7)
  const akhir = new Date(mulai)
  akhir.setDate(mulai.getDate() + 34)
  const bulanKini = kini.getMonth()
  const judul =
    mulai.getMonth() === akhir.getMonth()
      ? `${fmtBulan.format(mulai)} ${fmtTahun.format(mulai)}`
      : `${fmtBulan.format(mulai)} sampai ${fmtBulan.format(akhir)} ${fmtTahun.format(akhir)}`
  const sel = Array.from({ length: 35 }, (_, i) => {
    const d = new Date(mulai)
    d.setDate(mulai.getDate() + i)
    return d
  })
  const iniHari = hariIniIso()
  const acara = ACARA.map((a) => ({ ...a, iso: hariKe(a.sisa), status: statusDariSisa(a.sisa) as Status }))

  return (
    <div className="frag frag--kalender" aria-hidden="true" inert>
      <div className="fk__langganan">
        <div className="fk__label">Tautan langganan kalender</div>
        <div className="field-row">
          <span className="control fk__url">webcal://tenggat.mrramdn.com/api/kalender/k7Qe…4n.ics</span>
          <span className="btn btn--secondary">
            <Icon name="copy" />
            Salin
          </span>
        </div>
      </div>
      <div className="fk__kal">
        <div className="fk__kepala">
          <b>{judul}</b>
          <span>Kalender Tenggat</span>
        </div>
        <div className="fk__grid">
          {HARI.map((h) => (
            <span key={h} className="fk__hari">
              {h}
            </span>
          ))}
          {sel.map((d) => {
            const iso = hariIniIso(d)
            const di = acara.filter((a) => a.iso === iso)
            const luar = d.getMonth() !== bulanKini
            return (
              <div key={iso} className={`fk__sel${luar ? ' is-luar' : ''}${iso === iniHari ? ' is-kini' : ''}`}>
                <span className="fk__tgl num">{d.getDate()}</span>
                {di.map((a) => (
                  <span key={a.teks} className={`fk__acara st-${kelasStatus[a.status]}`}>
                    <span className="dot" />
                    <span className="fk__acara-teks">{a.teks}</span>
                  </span>
                ))}
              </div>
            )
          })}
        </div>
        <ul className="fk__agenda">
          {acara
            .filter((a) => a.sisa >= 0)
            .slice(0, 5)
            .map((a) => (
              <li key={a.teks}>
                <span className="fk__agenda-tgl num">{tanggalPendek(a.iso)}</span>
                <span className="fk__agenda-teks">{a.teks}</span>
                <span className="fk__agenda-sisa num">{frasaHari(a.sisa)}</span>
              </li>
            ))}
        </ul>
      </div>
    </div>
  )
}
