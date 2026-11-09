// First run: no assets yet. Typing a host starts "Ambil dari server" in the sheet.
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
        Belum ada aset. <span className="quiet">Mulai dari satu situs.</span>
      </h1>
      <p className="empty__lede">
        Ketik host situs web Anda. Tenggat membaca tanggal berakhir sertifikatnya langsung dari server dan memeriksanya
        ulang setiap 6 jam.
      </p>
      <form
        className="empty__form"
        onSubmit={(e) => {
          e.preventDefault()
          const h = host.trim()
          aksi.tambah({ jenis: 'sertifikat', host: h, ambil: h !== '' })
        }}
      >
        <label className="field__label" htmlFor="first-host">
          Host situs web
        </label>
        <div className="field-row">
          <label className="input-icon">
            <Icon name="globe" />
            <input
              id="first-host"
              className="control"
              placeholder="contoh: tokoanda.id"
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
            <Icon name="server" />
            Ambil dari server
          </button>
        </div>
        <div className="empty__alt">
          Untuk domain, token API, atau langganan, isi tanggalnya sendiri.
          <button type="button" className="btn btn--ghost is-sm" onClick={() => aksi.tambah({ jenis: 'domain' })}>
            <Icon name="plus" />
            Tambah manual
          </button>
        </div>
      </form>
      <div className="how">
        <div className="how__item">
          <b>Satu daftar</b>
          <span>Domain, sertifikat TLS, token API, dan langganan semua klien di satu tempat.</span>
        </div>
        <div className="how__item">
          <b>Pengingat di ponsel</b>
          <span>Unduh kalender (.ics), tenggat muncul di aplikasi kalender Anda.</span>
        </div>
        <div className="how__item">
          <b>Hanya nama dan tanggal</b>
          <span>Jangan simpan nilai token atau kata sandi.</span>
        </div>
      </div>
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
