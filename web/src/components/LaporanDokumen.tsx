// The monthly client report as a document: the same view in the owner's Laporan tab, on the public link and on
// paper. CetakLaporan puts a copy straight under <body> so the browser prints only the document (laporan.css).
import { createPortal } from 'react-dom'
import { bagianDurasi, durasi, type Laporan } from '../lib/api-laporan'
import { kelasStatus, labelJenis, labelStatus, rupiahBiasa, sisaHari, tanggal, urutanStatus } from '../lib/format'
import { Pill } from './ui'

export { durasi }
import '../styles/laporan.css'

const fmtDibuat = new Intl.DateTimeFormat('id-ID', {
  timeZone: 'Asia/Jakarta',
  day: 'numeric',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
})

/** "10 Nov 2026, 04.12 WIB" */
function waktuDibuat(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const p = Object.fromEntries(fmtDibuat.formatToParts(d).map((x) => [x.type, x.value]))
  return `${p.day} ${p.month} ${p.year}, ${p.hour}.${p.minute} WIB`
}

/** "1 sampai 30 November 2026" */
function periode(bulan: string, label: string): string {
  const [y, m] = bulan.split('-').map(Number)
  const akhir = new Date(y, m, 0).getDate()
  return `1 sampai ${akhir} ${label}`
}


/** A duration as a big figure: the numbers large, "jam" and "menit" in the label size. */
function DurasiAngka({ menit }: { menit: number }) {
  return (
    <>
      {bagianDurasi(menit).map(([n, s], i) => (
        <span key={s}>
          {i > 0 ? ' ' : ''}
          {n}
          <span className="lap__fig-satuan"> {s}</span>
        </span>
      ))}
    </>
  )
}

const jenisTeks = (j: string) => (j === 'tugas' ? 'Tugas rutin' : (labelJenis as Record<string, string>)[j] ?? j)

const labelSumber: Record<string, string> = {
  server: 'Dicek dari server',
  registri: 'Dicek dari registri domain',
  manual: 'Dicatat manual',
}

function Paragraf({ teks }: { teks: string }) {
  const bagian = teks
    .trim()
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
  return (
    <>
      {bagian.map((p, i) => (
        <p key={i}>{p}</p>
      ))}
    </>
  )
}

interface Props {
  laporan: Laporan
  /** The summary to show instead of laporan.catatan (the owner's text while it is being typed). */
  catatan?: string
  /** 'pemilik': the owner's preview, with a hint where the summary is still empty. */
  varian?: 'pemilik' | 'publik'
  /** Heading level of the title; sections use the next level. */
  tingkat?: 1 | 2
}

export function LaporanDokumen({ laporan: l, catatan, varian = 'publik', tingkat = 1 }: Props) {
  const H = tingkat === 1 ? 'h1' : 'h2'
  const S = tingkat === 1 ? 'h2' : 'h3'
  const r = l.ringkasan
  const teks = (catatan ?? l.catatan ?? '').trim()
  const perluTindakan = r.per_status.lewat + r.per_status.kritis + r.per_status.segera
  const totalStatus = urutanStatus.reduce((n, s) => n + r.per_status[s], 0)
  const biaya60 = l.jatuh_tempo.reduce((n, j) => n + (j.biaya ?? 0), 0)
  const menitDicatat = l.pekerjaan.reduce((n, p) => n + (p.durasi_menit ?? 0), 0)
  const entriBerdurasi = l.pekerjaan.filter((p) => p.durasi_menit).length
  // A month that has ended shows its own work and renewals; assets can only be shown as they are today.
  const lampau = l.lampau === true
  const perHariIni = l.per_tanggal ? tanggal(l.per_tanggal) : 'hari ini'

  return (
    <article className="lap" aria-label={`Laporan ${l.klien.nama}, ${l.label_bulan}`}>
      <header className="lap__kepala">
        <div className="lap__merek">
          <span className="lap__wordmark" aria-hidden="true">
            tenggat
          </span>
          <span>Laporan bulanan</span>
        </div>
        <H className="lap__judul">{l.klien.nama}</H>
        <p className="lap__bulan">{l.label_bulan}</p>
        <dl className="lap__meta">
          <div>
            <dt>Disusun oleh</dt>
            <dd>{l.penyusun.nama}</dd>
          </div>
          <div>
            <dt>Periode</dt>
            <dd>{periode(l.bulan, l.label_bulan)}</dd>
          </div>
          <div>
            <dt>Dibuat</dt>
            <dd>{waktuDibuat(l.dibuat)}</dd>
          </div>
        </dl>
      </header>

      {teks ? (
        <section className="lap__bagian lap__ringkasan" aria-label="Ringkasan">
          <S className="lap__sub">Ringkasan</S>
          <div className="lap__prosa">
            <Paragraf teks={teks} />
          </div>
        </section>
      ) : varian === 'pemilik' ? (
        <p className="lap__kosong-ringkasan">
          Ringkasan untuk klien belum ditulis. Bagian ini tidak tampil di laporan sampai Anda mengisinya.
        </p>
      ) : null}

      <section className="lap__bagian" aria-label="Angka bulan ini">
        <S className="sr-only">Angka bulan ini</S>
        <div className={lampau ? 'lap__angka is-lampau' : 'lap__angka'}>
          <div className="lap__fig">
            <span className="lap__fig-label">Pekerjaan</span>
            <span className="lap__fig-n num">{r.pekerjaan}</span>
            <span className="lap__fig-sub">entri bulan ini</span>
          </div>
          <div className="lap__fig">
            <span className="lap__fig-label">Waktu kerja</span>
            <span className="lap__fig-n num is-durasi">
              <DurasiAngka menit={r.menit} />
            </span>
            <span className="lap__fig-sub">
              {r.menit > 0 ? `dari ${entriBerdurasi} entri` : 'tidak ada durasi tercatat'}
            </span>
          </div>
          {lampau ? (
            <div className="lap__fig">
              <span className="lap__fig-label">Perpanjangan</span>
              <span className="lap__fig-n num">{l.perpanjangan.length}</span>
              <span className="lap__fig-sub">bulan ini</span>
            </div>
          ) : (
            <>
              <div className="lap__fig">
                <span className="lap__fig-label">Aset</span>
                <span className="lap__fig-n num">{r.jumlah_aset}</span>
                <span className="lap__fig-sub">
                  {r.jumlah_aset === 0 ? 'belum ada aset' : perluTindakan === 0 ? 'semua aman' : `${perluTindakan} perlu tindakan`}
                </span>
              </div>
              <div className="lap__fig">
                <span className="lap__fig-label">Biaya 30 hari</span>
                <span className="lap__fig-n num">{r.biaya_30_hari > 0 ? rupiahBiasa(r.biaya_30_hari) : 'Tidak ada'}</span>
                <span className="lap__fig-sub">perpanjangan jatuh tempo</span>
              </div>
            </>
          )}
        </div>
        {totalStatus > 0 && !lampau ? (
          <div className="lap__status">
            <div className="lap__bar" aria-hidden="true">
              {urutanStatus.map((s) =>
                r.per_status[s] > 0 ? (
                  <span
                    key={s}
                    className={`lap__bar-seg bg-${kelasStatus[s]}`}
                    style={{ flexGrow: r.per_status[s] }}
                  />
                ) : null,
              )}
            </div>
            <ul className="lap__legenda" aria-label="Aset per status">
              {urutanStatus.map((s) => (
                <li key={s} className={r.per_status[s] === 0 ? 'is-nol' : undefined}>
                  <span className={`dot st-${kelasStatus[s]}`} />
                  {labelStatus[s]}
                  <b className="num">{r.per_status[s]}</b>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </section>

      <section className="lap__bagian">
        <div className="lap__judul-bagian">
          <S className="lap__sub">Pekerjaan bulan ini</S>
          {l.pekerjaan.length > 0 ? (
            <span className="lap__total">
              {l.pekerjaan.length} entri{menitDicatat > 0 ? `, ${durasi(menitDicatat)}` : ''}
            </span>
          ) : null}
        </div>
        {l.pekerjaan.length === 0 ? (
          <p className="lap__kosong">Tidak ada pekerjaan yang dicatat bulan ini.</p>
        ) : (
          <table className="lap-tabel t-kerja">
            <thead>
              <tr>
                <th scope="col" className="k-tgl">Tanggal</th>
                <th scope="col">Pekerjaan</th>
                <th scope="col" className="k-angka">Durasi</th>
              </tr>
            </thead>
            <tbody>
              {l.pekerjaan.map((p, i) => (
                <tr key={i}>
                  <td className="k-tgl num">{tanggal(p.tanggal)}</td>
                  <td className="k-utama">
                    <span className="lap__teks">{p.teks}</span>
                    {p.jenis === 'tugas_selesai' ? <span className="lap__kecil">Tugas rutin</span> : null}
                  </td>
                  <td className="k-angka num">{p.durasi_menit ? durasi(p.durasi_menit) : <span className="k-kosong">-</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="lap__bagian">
        <div className="lap__judul-bagian">
          <S className="lap__sub">Perpanjangan bulan ini</S>
        </div>
        {l.perpanjangan.length === 0 ? (
          <p className="lap__kosong">Tidak ada aset yang diperpanjang bulan ini.</p>
        ) : (
          <table className="lap-tabel t-panjang">
            <thead>
              <tr>
                <th scope="col" className="k-tgl">Tanggal</th>
                <th scope="col">Keterangan</th>
              </tr>
            </thead>
            <tbody>
              {l.perpanjangan.map((p, i) => (
                <tr key={i}>
                  <td className="k-tgl num">{tanggal(p.tanggal)}</td>
                  <td className="k-utama">
                    <span className="lap__teks">{p.teks}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      {lampau ? null : (
        <section className="lap__bagian">
          <div className="lap__judul-bagian">
            <S className="lap__sub">Jatuh tempo 60 hari ke depan</S>
            {biaya60 > 0 ? <span className="lap__total">Perkiraan biaya {rupiahBiasa(biaya60)}</span> : null}
          </div>
          {l.jatuh_tempo.length === 0 ? (
            <p className="lap__kosong">Tidak ada aset atau tugas yang jatuh tempo dalam 60 hari ke depan.</p>
          ) : (
            <table className="lap-tabel t-tempo">
              <thead>
                <tr>
                  <th scope="col" className="k-tgl">Tanggal</th>
                  <th scope="col">Aset atau tugas</th>
                  <th scope="col" className="k-sisa">Sisa</th>
                  <th scope="col" className="k-status">Status</th>
                  <th scope="col" className="k-angka">Biaya</th>
                </tr>
              </thead>
              <tbody>
                {l.jatuh_tempo.map((j, i) => (
                  <tr key={i}>
                    <td className="k-tgl num">{tanggal(j.tanggal)}</td>
                    <td className="k-utama">
                      <span className="lap__teks lap__nama">{j.nama}</span>
                      <span className="lap__kecil">{jenisTeks(j.jenis)}</span>
                    </td>
                    <td className={j.sisa_hari < 0 ? 'k-sisa num is-lewat' : 'k-sisa num'}>{sisaHari(j.sisa_hari)}</td>
                    <td className="k-status">
                      <Pill status={j.status} />
                    </td>
                    <td className="k-angka num" data-label="Biaya">
                      {j.biaya != null ? rupiahBiasa(j.biaya) : <span className="k-kosong">-</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}

      <section className="lap__bagian">
        <div className="lap__judul-bagian">
          <S className="lap__sub">{lampau ? `Daftar aset per ${perHariIni}` : 'Daftar aset'}</S>
          {l.aset.length > 0 ? <span className="lap__total">{l.aset.length} aset</span> : null}
        </div>
        {lampau && l.aset.length > 0 ? (
          <p className="lap__catatan-lampau">
            Tanggal berakhir dan status di daftar ini adalah keadaan {perHariIni}, bukan akhir {l.label_bulan}.
          </p>
        ) : null}
        {l.aset.length === 0 ? (
          <p className="lap__kosong">Belum ada aset yang dicatat untuk klien ini.</p>
        ) : (
          <table className="lap-tabel t-aset">
            <thead>
              <tr>
                <th scope="col">Aset</th>
                <th scope="col" className="k-jenis">Jenis</th>
                <th scope="col" className="k-tgl">Berakhir</th>
                <th scope="col" className="k-status">Status</th>
                <th scope="col" className="k-angka">Biaya</th>
              </tr>
            </thead>
            <tbody>
              {l.aset.map((a, i) => (
                <tr key={i}>
                  <td className="k-utama">
                    <span className="lap__teks lap__nama">{a.nama}</span>
                    <span className="lap__kecil">
                      {a.host && a.host !== a.nama ? `${a.host} · ` : ''}
                      {labelSumber[a.sumber] ?? ''}
                    </span>
                  </td>
                  <td className="k-jenis">{jenisTeks(a.jenis)}</td>
                  <td className="k-tgl num">{tanggal(a.kedaluwarsa)}</td>
                  <td className="k-status">
                    <Pill status={a.status} />
                  </td>
                  <td className="k-angka num" data-label="Biaya">
                    {a.biaya != null ? rupiahBiasa(a.biaya) : <span className="k-kosong">-</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <footer className="lap__kaki">
        <p>
          Status: Lewat berarti sudah berakhir, 7 hari dan 30 hari berarti berakhir dalam rentang itu. Semua tanggal
          memakai waktu Indonesia Barat (WIB).
        </p>
        <p>Disusun dengan Tenggat.</p>
      </footer>
    </article>
  )
}

/** A copy of the report directly under <body>, shown only when printing (Cetak / Simpan PDF or Ctrl+P). */
export function CetakLaporan({ laporan, catatan }: { laporan: Laporan; catatan?: string }) {
  return createPortal(
    <div className="lap-cetak" aria-hidden="true">
      <LaporanDokumen laporan={laporan} catatan={catatan} varian="publik" />
    </div>,
    document.body,
  )
}

/** Prints the report with a title the browser offers as the PDF's file name. */
export function cetak(laporan: Laporan) {
  const judulLama = document.title
  document.title = `Laporan ${laporan.klien.nama} ${laporan.label_bulan}`
  const pulihkan = () => {
    document.title = judulLama
    window.removeEventListener('afterprint', pulihkan)
  }
  window.addEventListener('afterprint', pulihkan)
  window.print()
}
