// Phone asset list: rows grouped by urgency. Tapping a row opens it for editing; the primary action sits on the row.
import type { Item, Status } from '../lib/api'
import { labelJenisPendek, labelUlangItem, lalu, tanggalPendek, uang } from '../lib/format'
import { catatanKurs, teksKiraRupiah, useKurs } from '../lib/kurs'
import { Icon, ikonJenis } from './Icon'
import type { Aksi } from './types'
import { Days, Pill } from './ui'

function PhoneRow({
  it,
  aksi,
  tampilKlien,
  kira,
  catatan,
}: {
  it: Item
  aksi: Aksi
  tampilKlien: boolean
  kira: string | null
  catatan?: string
}) {
  const bisaCek = !!it.host && (it.jenis === 'sertifikat' || it.sumber === 'registri')
  const tagihan = it.jenis === 'tagihan'
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
            {it.biaya ? (
              <>
                {' · '}
                <span className="num">{uang(it.biaya, it.mata_uang)}</span>
                {kira ? (
                  <span className="kira num" title={catatan}>
                    {' '}
                    {kira}
                  </span>
                ) : null}
              </>
            ) : null}
            {it.ulang_jumlah && it.ulang_satuan ? ` · ${labelUlangItem(it.ulang_jumlah, it.ulang_satuan)}` : ''}
            {tampilKlien && it.klien ? ` · ${it.klien}` : ''}
          </span>
        </div>
      </div>
      <div className="ph-row__end">
        <Pill status={it.status} />
        <span className="cell num">{tanggalPendek(it.kedaluwarsa)}</span>
      </div>
      {it.galat_cek ? (
        <div className="ph-row__fail">
          <Icon name="alert" />
          <div>
            Cek gagal{it.terakhir_dicek ? ` ${lalu(it.terakhir_dicek)}` : ''}. <span>{it.galat_cek}</span>
          </div>
        </div>
      ) : null}
      <div className="ph-row__act">
        {bisaCek ? (
          <button
            type="button"
            className={cek ? 'btn btn--secondary is-lg is-busy' : 'btn btn--secondary is-lg'}
            aria-busy={cek}
            onClick={() => aksi.cekUlang(it)}
          >
            <Icon name={cek ? 'loader' : 'refresh'} className={cek ? 'spin' : undefined} />
            {cek ? 'Memeriksa' : 'Cek ulang'}
          </button>
        ) : tagihan ? (
          <button type="button" className="btn btn--secondary is-lg" onClick={() => aksi.perpanjang(it)}>
            <Icon name="check" />
            Tandai dibayar
          </button>
        ) : (
          <button type="button" className="btn btn--secondary is-lg" onClick={() => aksi.perpanjang(it)}>
            <Icon name="calendar" />
            Perpanjang
          </button>
        )}
      </div>
    </div>
  )
}

const GRUP: { judul: string; status: Status[] }[] = [
  { judul: 'Perlu tindakan', status: ['lewat', 'kritis'] },
  { judul: '30 hari ke depan', status: ['segera'] },
  { judul: 'Aman', status: ['aman'] },
]

export function DaftarAsetPhone({
  rows,
  aksi,
  tampilKlien = true,
  catatanSudahTampil = false,
}: {
  rows: Item[]
  aksi: Aksi
  tampilKlien?: boolean
  catatanSudahTampil?: boolean
}) {
  const kurs = useKurs()
  const catatan = catatanKurs(kurs)
  let catatanDipakai = catatanSudahTampil
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
            {isi.map((it) => {
              const kira = teksKiraRupiah(it.biaya, it.mata_uang, kurs)
              const bawaCatatan = !catatanDipakai && kira != null
              if (bawaCatatan) catatanDipakai = true
              return (
                <PhoneRow
                  key={it.id}
                  it={it}
                  aksi={aksi}
                  tampilKlien={tampilKlien}
                  kira={kira}
                  catatan={bawaCatatan ? catatan : undefined}
                />
              )
            })}
          </section>
        )
      })}
    </>
  )
}
