// The product beside the sign-in form: a slice of the asset list, drawn with the
// app's own pill and day count from fictional data. Decorative, so hidden from
// assistive tech and never focusable.
import type { Jenis, Status } from '../lib/api'
import { Icon, ikonJenis } from './Icon'
import { Days, Pill } from './ui'

interface Contoh {
  nama: string
  jenis: Jenis
  sub: string
  klien: string
  sisa: number
}

const CONTOH: Contoh[] = [
  { nama: 'Lisensi font editorial', jenis: 'lainnya', sub: 'Lainnya · Rp 450.000', klien: 'Studio Arsitek Rana', sisa: -3 },
  { nama: 'Domain tokokuemawar.id', jenis: 'domain', sub: 'Domain · Rp 285.000', klien: 'Toko Kue Mawar', sisa: 4 },
  { nama: 'Token GitHub Actions deploy', jenis: 'token', sub: 'Token API', klien: 'Pribadi', sisa: 6 },
  { nama: 'Langganan VPS 2 vCPU', jenis: 'langganan', sub: 'Langganan · Rp 189.000', klien: 'Pribadi', sisa: 22 },
  { nama: 'example.com', jenis: 'sertifikat', sub: "Sertifikat TLS · Let's Encrypt R11", klien: 'Contoh publik', sisa: 46 },
  { nama: 'wikipedia.org', jenis: 'sertifikat', sub: "Sertifikat TLS · Let's Encrypt E6", klien: 'Contoh publik', sisa: 54 },
  { nama: 'Domain kopisenja.com', jenis: 'domain', sub: 'Domain · Rp 210.000', klien: 'Kopi Senja', sisa: 72 },
  { nama: 'Token Cloudflare DNS', jenis: 'token', sub: 'Token API', klien: 'Pribadi', sisa: 181 },
  { nama: 'Google Workspace', jenis: 'langganan', sub: 'Langganan · Rp 1.680.000', klien: 'Studio Arsitek Rana', sisa: 214 },
  { nama: 'tokokuemawar.id', jenis: 'sertifikat', sub: "Sertifikat TLS · Let's Encrypt R10", klien: 'Toko Kue Mawar', sisa: 238 },
  { nama: 'Domain studiorana.co.id', jenis: 'domain', sub: 'Domain · Rp 320.000', klien: 'Studio Arsitek Rana', sisa: 301 },
]

const URUTAN: Status[] = ['lewat', 'kritis', 'segera', 'aman']
const fmtTanggal = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })
const fmtPendek = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short' })

function status(n: number): Status {
  if (n < 0) return 'lewat'
  if (n <= 7) return 'kritis'
  if (n <= 30) return 'segera'
  return 'aman'
}

function hariKe(n: number): Date {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  d.setDate(d.getDate() + n)
  return d
}

/** "7 Nov", "14–16 Nov", "Des–Mei": the same short ranges as the overview strip. */
function rentang(sisa: number[]): string {
  if (sisa.length === 0) return ''
  const a = hariKe(Math.min(...sisa))
  const b = hariKe(Math.max(...sisa))
  if (a.getTime() === b.getTime()) return fmtPendek.format(a)
  if (a.getMonth() === b.getMonth() && a.getFullYear() === b.getFullYear()) return `${a.getDate()}–${fmtPendek.format(b)}`
  const bulan = new Intl.DateTimeFormat('id-ID', { month: 'short' })
  return `${bulan.format(a)}–${bulan.format(b)}`
}

export function MasukPratinjau() {
  return (
    <div className="pv" aria-hidden="true" inert>
      <div className="pv__top">
        <span className="pv__crumbs">
          <b>Ringkasan</b>
          <span>/</span>
          <span>Semua klien</span>
        </span>
      </div>
      <div className="pv__body">
        <div className="pv__stats">
          {URUTAN.map((st) => {
            const isi = CONTOH.filter((c) => status(c.sisa) === st).map((c) => c.sisa)
            return (
              <div key={st} className="pv__stat">
                <Pill status={st} />
                <span className="pv__stat-row">
                  <span className="stat__n num">{isi.length}</span>
                  <span className="stat__note num">{rentang(isi)}</span>
                </span>
              </div>
            )
          })}
        </div>
        <div className="pv__rows">
          <div className="pv__row pv__row--head">
            <span>Sisa</span>
            <span>Aset</span>
            <span className="pv__klien">Klien</span>
            <span>Berakhir</span>
          </div>
          {CONTOH.map((c) => (
            <div key={c.nama} className="pv__row">
              <div className="sisa">
                <Days n={c.sisa} />
                <Pill status={status(c.sisa)} />
              </div>
              <div className="asset">
                <div className="asset__name">{c.nama}</div>
                <div className="asset__sub">
                  <Icon name={ikonJenis[c.jenis]} />
                  <span>{c.sub}</span>
                </div>
              </div>
              <div className="cell pv__klien">{c.klien}</div>
              <div className="cell num">{fmtTanggal.format(hariKe(c.sisa))}</div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
