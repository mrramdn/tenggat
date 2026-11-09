// Phone asset list: rows grouped by urgency. Tapping a row opens it for editing.
import type { Item, Status } from '../lib/api'
import { labelJenisPendek, lalu, tanggalPendek } from '../lib/format'
import { Icon, ikonJenis } from './Icon'
import type { Aksi } from './types'
import { Days, Pill } from './ui'

function PhoneRow({ it, aksi, tampilKlien }: { it: Item; aksi: Aksi; tampilKlien: boolean }) {
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
            {tampilKlien && it.klien ? ` · ${it.klien}` : ''}
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
            <button
              type="button"
              className={cek ? 'btn btn--secondary is-lg is-busy' : 'btn btn--secondary is-lg'}
              aria-busy={cek}
              onClick={() => aksi.cekUlang(it)}
            >
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

export function DaftarAsetPhone({ rows, aksi, tampilKlien = true }: { rows: Item[]; aksi: Aksi; tampilKlien?: boolean }) {
  return (
    <>
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
              <PhoneRow key={it.id} it={it} aksi={aksi} tampilKlien={tampilKlien} />
            ))}
          </section>
        )
      })}
    </>
  )
}
