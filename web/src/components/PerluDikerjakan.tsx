// "Perlu dikerjakan" on /app: tasks of every client that are overdue or due within 14 days. Each row opens the
// client's Tugas tab.
import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { kunciTugas, labelUlang, tugasApi } from '../lib/api-tugas'
import { sisaHari, tanggal } from '../lib/format'
import { Tautan } from '../lib/rute'
import { Icon } from './Icon'
import { Ikon } from './klien/ikon'
import { pesanGalat } from './Toasts'
import { Pill } from './ui'
import '../styles/tugas.css'

const RENTANG = 14
const AWAL = 6

export function PerluDikerjakan() {
  const id = useId()
  const [semua, setSemua] = useState(false)
  const q = useQuery({ queryKey: kunciTugas.dalamHari(RENTANG), queryFn: () => tugasApi.daftar({ dalamHari: RENTANG }) })
  const list = [...(q.data ?? [])].sort((a, b) => a.jatuh_tempo.localeCompare(b.jatuh_tempo))
  const tampil = semua ? list : list.slice(0, AWAL)
  const lewat = list.filter((t) => t.status === 'lewat').length

  let isi
  if (q.isPending) {
    isi = (
      <div className="pd__list" aria-busy="true" aria-label="Memuat tugas">
        {[0, 1, 2].map((i) => (
          <div key={i} className="pd__row is-sk">
            <span className="sk" style={{ width: 64, height: 24 }} />
            <span className="sk" style={{ width: '50%', height: 16 }} />
          </div>
        ))}
      </div>
    )
  } else if (q.isError) {
    isi = (
      <div className="pd__msg" role="alert">
        <span>Tugas tidak dapat dimuat. {pesanGalat(q.error)}</span>
        <button type="button" className="btn btn--secondary is-sm" onClick={() => void q.refetch()}>
          Coba lagi
        </button>
      </div>
    )
  } else if (list.length === 0) {
    isi = (
      <div className="pd__msg">
        <Icon name="check" />
        <span>Tidak ada tugas yang lewat atau jatuh tempo dalam {RENTANG} hari.</span>
      </div>
    )
  } else {
    isi = (
      <>
        <ul className="pd__list">
          {tampil.map((t) => {
            const isi = (
              <>
                <Pill status={t.status} />
                <span className="pd__main">
                  <span className="pd__judul">{t.judul}</span>
                  <span className="pd__sub">
                    <span className="pd__klien">{t.klien || 'Tanpa klien'}</span>
                    <span className="pd__ulang"> · {labelUlang(t.ulang_jumlah, t.ulang_satuan)}</span>
                  </span>
                </span>
                <span className="pd__due num">
                  <span className={t.sisa_hari < 0 ? 'pd__sisa is-lewat' : 'pd__sisa'}>{sisaHari(t.sisa_hari)}</span>
                  <span className="pd__tgl">{tanggal(t.jatuh_tempo)}</span>
                </span>
              </>
            )
            return (
              <li key={t.id}>
                {t.klien_id ? (
                  <Tautan ke={`/app/klien/${t.klien_id}/tugas`} className="pd__row">
                    {isi}
                    <Ikon name="chevron-right" className="pd__go" />
                  </Tautan>
                ) : (
                  <div className="pd__row">{isi}</div>
                )}
              </li>
            )
          })}
        </ul>
        {list.length > AWAL ? (
          <button type="button" className="btn btn--ghost is-sm pd__more" aria-expanded={semua} onClick={() => setSemua((s) => !s)}>
            {semua ? 'Tampilkan lebih sedikit' : `Tampilkan ${list.length - AWAL} lainnya`}
          </button>
        ) : null}
      </>
    )
  }

  return (
    <section className="pd" aria-labelledby={id}>
      <div className="pd__head">
        <h2 className="pd__title" id={id}>
          Perlu dikerjakan
        </h2>
        <span className="pd__note num">
          {q.data && list.length > 0
            ? `${list.length} tugas${lewat ? `, ${lewat} lewat jatuh tempo` : ''}`
            : `Tugas lewat atau jatuh tempo dalam ${RENTANG} hari`}
        </span>
      </div>
      {isi}
    </section>
  )
}
