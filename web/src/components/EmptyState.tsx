// First run: no assets yet. Typing a domain opens the sheet and reads its date from the registry.
import { useState } from 'react'
import { hariIniIso, tanggalPanjang } from '../lib/format'
import { Icon } from './Icon'
import { Timeline } from './Timeline'
import type { Aksi } from './types'
import { Pill } from './ui'
import { urutanStatus } from '../lib/format'

export function EmptyState({ aksi, phone = false }: { aksi: Aksi; phone?: boolean }) {
  const [host, setHost] = useState('')
  const isi = (
    <div>
      <div className="eyebrow">{tanggalPanjang(hariIniIso())}</div>
      <h1 className="display">
        Belum ada aset.
      </h1>
      <p className="empty__lede">
        Ketik nama domain situs Anda. Tenggat membaca tanggal berakhir domainnya dari registri dan mengeceknya ulang
        setiap 6 jam. Sertifikat TLS dibaca dengan cara yang sama dari servernya.
      </p>
      <form
        className="empty__form"
        onSubmit={(e) => {
          e.preventDefault()
          const h = host.trim()
          aksi.tambah({ jenis: 'domain', host: h, ambil: h !== '' })
        }}
      >
        <label className="field__label" htmlFor="first-host">
          Domain situs web
        </label>
        <div className="field-row">
          <label className="input-icon">
            <Icon name="globe" />
            <input
              id="first-host"
              className="control"
              placeholder="misalnya tokokuemawar.id"
              autoComplete="off"
              autoCapitalize="off"
              spellCheck={false}
              inputMode="url"
              autoFocus={!phone}
              value={host}
              onChange={(e) => setHost(e.target.value)}
            />
          </label>
          <button type="submit" className="btn btn--primary">
            <Icon name="database" />
            Ambil dari registri
          </button>
        </div>
        <div className="empty__alt">
          <button type="button" className="btn btn--ghost is-sm" onClick={() => aksi.tambah({ jenis: 'sertifikat' })}>
            <Icon name="plus" />
            Tambah jenis aset lain
          </button>
        </div>
      </form>
    </div>
  )

  if (phone) return <div className="ph-start">{isi}</div>

  return (
    <div className="empty">
      {isi}
      <div className="ghost-tl" aria-label="Pratinjau ringkasan">
        <div className="tl__head">
          <b>Ringkasan</b>
          <span>terisi setelah aset pertama</span>
        </div>
        <div className="ghost-stats">
          {urutanStatus.map((s) => (
            <div key={s} className="ghost-stat">
              <Pill status={s} />
              <span className="stat__n num">0</span>
            </div>
          ))}
        </div>
        <Timeline items={[]} ghost />
      </div>
    </div>
  )
}
