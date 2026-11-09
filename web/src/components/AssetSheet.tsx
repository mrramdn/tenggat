// Sheet "Tambah aset" / "Ubah aset". For a certificate the user types only the
// host; "Ambil dari server" shows what the server sends before saving. A domain
// works the same way with "Ambil dari registri", or takes a date typed by hand.
import { useMutation } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { ApiError, api, type Item, type ItemInput, type Jenis, type Klien, type PratinjauDomain, type PratinjauSertifikat, type Status, type TLSInfo } from '../lib/api'
import { angka, frasaHari, labelJenis, labelJenisPendek, lalu, tanggal, urutanJenis } from '../lib/format'
import { useModal } from '../hooks/useModal'
import { tautanPengingat } from '../lib/pengingat'
import { pesanGalat } from './Toasts'
import { Icon } from './Icon'
import { KlienPicker, type PilihanKlien } from './KlienPicker'
import type { BukaTambah } from './types'
import { Pill } from './ui'

export type ModeSheet = { mode: 'tambah'; awal: BukaTambah } | { mode: 'ubah'; item: Item; fokus?: 'kedaluwarsa' }

interface Props {
  buka: ModeSheet
  klien: Klien[]
  phone: boolean
  onClose: () => void
  onSaved: (it: Item, baru: boolean) => void
  onHapus: (it: Item) => void
}

type HasilOk = { status: 'ok'; host: string; kedaluwarsa: string; sisa: number; st: Status; meta: string; judul: string }

type Hasil =
  | { status: 'kosong' }
  | { status: 'memuat'; host: string; jenis: 'sertifikat' | 'domain' }
  | (HasilOk & { jenis: 'sertifikat'; tls: TLSInfo })
  | (HasilOk & { jenis: 'domain'; domain: string; registrar: string; server: string; galat: string | null })

const fmtWaktu = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })
const tanggalDari = (iso: string) => fmtWaktu.format(new Date(iso))

function dariPratinjau(p: PratinjauSertifikat): Hasil {
  return {
    status: 'ok',
    jenis: 'sertifikat',
    host: p.host,
    kedaluwarsa: p.kedaluwarsa,
    sisa: p.sisa_hari,
    st: p.status,
    tls: p.tls,
    meta: 'port 443 · baru saja',
    judul: 'Sertifikat terbaca dari server',
  }
}

function dariDomain(p: PratinjauDomain): Hasil {
  return {
    status: 'ok',
    jenis: 'domain',
    host: p.host,
    domain: p.domain,
    kedaluwarsa: p.kedaluwarsa,
    sisa: p.sisa_hari,
    st: p.status,
    registrar: p.registri.registrar,
    server: p.registri.server,
    galat: null,
    meta: 'RDAP · baru saja',
    judul: 'Tanggal terbaca dari registri',
  }
}

function dariItem(it: Item): Hasil {
  if (it.jenis === 'domain' && it.sumber === 'registri' && it.registri && it.host) {
    return {
      status: 'ok',
      jenis: 'domain',
      host: it.host,
      domain: it.registri.domain,
      kedaluwarsa: it.kedaluwarsa,
      sisa: it.sisa_hari,
      st: it.status,
      registrar: it.registri.registrar,
      server: it.registri.server,
      galat: it.galat_cek,
      meta: it.terakhir_dicek ? `RDAP · ${lalu(it.terakhir_dicek)}` : 'RDAP',
      judul: 'Tanggal terakhir dibaca dari registri',
    }
  }
  if (it.jenis !== 'sertifikat' || !it.tls || !it.host) return { status: 'kosong' }
  return {
    status: 'ok',
    jenis: 'sertifikat',
    host: it.host,
    kedaluwarsa: it.kedaluwarsa,
    sisa: it.sisa_hari,
    st: it.status,
    tls: it.tls,
    meta: it.terakhir_dicek ? `port 443 · ${lalu(it.terakhir_dicek)}` : 'port 443',
    judul: 'Sertifikat terakhir dibaca dari server',
  }
}

function FieldError({ id, pesan }: { id: string; pesan?: string }) {
  if (!pesan) return null
  return (
    <div className="field__error" id={id}>
      <Icon name="alert" />
      <span>{pesan}</span>
    </div>
  )
}

function Result({ hasil }: { hasil: Hasil }) {
  if (hasil.status === 'kosong') return null
  if (hasil.status === 'memuat') {
    return (
      <div className="result" aria-busy="true">
        <div className="result__head is-loading">
          <Icon name="loader" className="spin" />
          {hasil.jenis === 'domain' ? `Menanyakan ${hasil.host} ke registri` : `Menghubungi ${hasil.host} di port 443`}
        </div>
        <div className="result__main">
          <span className="sk" style={{ width: 200, height: 28 }} />
          <span className="sk" style={{ width: '100%', height: 6 }} />
          <span className="sk" style={{ width: '70%', height: 14 }} />
          <span className="sk" style={{ width: '55%', height: 14 }} />
        </div>
      </div>
    )
  }
  if (hasil.jenis === 'domain') return <ResultDomain hasil={hasil} />
  const { tls } = hasil
  const mulai = new Date(tls.berlaku_sejak).getTime()
  const akhir = new Date(tls.berakhir).getTime()
  const hari = 86_400_000
  const total = Math.max(1, Math.round((akhir - mulai) / hari))
  const pakai = Math.min(total, Math.max(0, Math.round((Date.now() - mulai) / hari)))
  return (
    <div className="result">
      <div className="result__head">
        <Icon name="check" />
        {hasil.judul}
        <span className="meta">{hasil.meta}</span>
      </div>
      <div className="result__main">
        <div className="result__date">
          <span className="big num">{tanggal(hasil.kedaluwarsa)}</span>
          <Pill status={hasil.st} />
          <span className={hasil.sisa < 0 ? 'days__u num is-lewat' : 'days__u num'}>{frasaHari(hasil.sisa)}</span>
        </div>
        <div className="validity">
          <div
            className="validity__bar"
            role="meter"
            aria-label="Masa berlaku terpakai"
            aria-valuemin={0}
            aria-valuemax={total}
            aria-valuenow={pakai}
          >
            <div className="validity__fill" style={{ width: `${(pakai / total) * 100}%` }} />
          </div>
          <div className="validity__legend num">
            <span>{tanggalDari(tls.berlaku_sejak)}</span>
            <span>
              {pakai} dari {total} hari terpakai
            </span>
            <span>{tanggal(hasil.kedaluwarsa)}</span>
          </div>
        </div>
        <dl className="dl">
          <dt>Penerbit</dt>
          <dd>{tls.penerbit || 'Tidak tercantum'}</dd>
          <dt>Berlaku sejak</dt>
          <dd className="num">{tanggalDari(tls.berlaku_sejak)}</dd>
          <dt>Berakhir</dt>
          <dd className="num">{tanggal(hasil.kedaluwarsa)}</dd>
          <dt>Sumber</dt>
          <dd>Dari server, dicek ulang setiap 6 jam</dd>
          {tls.rantai_valid ? null : (
            <>
              <dt>Rantai</dt>
              <dd className="is-warn">Tidak valid{tls.galat_rantai ? `: ${tls.galat_rantai}` : ''}</dd>
            </>
          )}
        </dl>
      </div>
    </div>
  )
}

function ResultDomain({ hasil }: { hasil: Extract<Hasil, { jenis: 'domain'; status: 'ok' }> }) {
  return (
    <div className="result">
      <div className="result__head">
        <Icon name="check" />
        {hasil.judul}
        <span className="meta">{hasil.meta}</span>
      </div>
      <div className="result__main">
        <div className="result__date">
          <span className="big num">{tanggal(hasil.kedaluwarsa)}</span>
          <Pill status={hasil.st} />
          <span className={hasil.sisa < 0 ? 'days__u num is-lewat' : 'days__u num'}>{frasaHari(hasil.sisa)}</span>
        </div>
        <dl className="dl">
          {hasil.domain && hasil.domain !== hasil.host ? (
            <>
              <dt>Domain</dt>
              <dd>{hasil.domain}</dd>
            </>
          ) : null}
          <dt>Registrar</dt>
          <dd>{hasil.registrar || 'Tidak tercantum'}</dd>
          <dt>Registri</dt>
          <dd>{hasil.server || 'Tidak tercantum'}</dd>
          <dt>Sumber</dt>
          <dd>Dari registri, dicek ulang setiap 6 jam</dd>
          {hasil.galat ? (
            <>
              <dt>Cek terakhir</dt>
              <dd className="is-warn">{hasil.galat}</dd>
            </>
          ) : null}
        </dl>
      </div>
    </div>
  )
}

export function AssetSheet({ buka, klien, phone, onClose, onSaved, onHapus }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  useModal(ref, onClose)
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`

  const item = buka.mode === 'ubah' ? buka.item : null
  const awal = buka.mode === 'tambah' ? buka.awal : null
  const [jenis, setJenis] = useState<Jenis>(item?.jenis ?? awal?.jenis ?? 'sertifikat')
  const [host, setHost] = useState(item?.host ?? awal?.host ?? '')
  const [nama, setNama] = useState(item ? item.nama : '')
  const [namaDisentuh, setNamaDisentuh] = useState(!!item && item.nama !== item.host && item.nama !== item.registri?.domain)
  const [klienNya, setKlien] = useState<PilihanKlien>(
    item ? { id: item.klien_id, nama: item.klien } : { id: awal?.klien?.id ?? null, nama: awal?.klien?.nama ?? '' },
  )
  const [kedaluwarsa, setKedaluwarsa] = useState(item && item.sumber === 'manual' ? item.kedaluwarsa : '')
  // A domain is read from the registry unless the user chose to type its date.
  const [tanggalSendiri, setTanggalSendiri] = useState(!!item && item.jenis === 'domain' && item.sumber === 'manual')
  const [biaya, setBiaya] = useState(item?.biaya != null ? String(item.biaya) : '')
  const [hasil, setHasil] = useState<Hasil>(item ? dariItem(item) : { status: 'kosong' })
  const [galat, setGalat] = useState<Record<string, string>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)

  const sertifikat = jenis === 'sertifikat'
  const klienBaru = !klienNya.id && klienNya.nama.trim() !== ''
  const domain = jenis === 'domain'
  const pakaiHost = sertifikat || domain
  const gagalBaca = pakaiHost && !!galat.host && host.trim() !== '' && hasil.status !== 'memuat'
  // A certificate the server could not read can still be saved with a date typed by hand.
  const cadangan = sertifikat && gagalBaca
  // The same for a domain the registry could not answer; it then stays a domain with a manual date.
  const domainManual = domain && (tanggalSendiri || gagalBaca)
  const dariRegistri = domain && !domainManual
  const hasilDomain = hasil.status === 'ok' && hasil.jenis === 'domain' ? hasil : null
  const namaTampil = namaDisentuh || !pakaiHost ? nama : (hasilDomain?.domain ?? host.trim())

  function hapusGalat(...k: string[]) {
    setGalat((g) => {
      const n = { ...g }
      for (const key of k) delete n[key]
      return n
    })
    setGalatUmum(null)
  }

  const permintaan = useRef(0)
  async function ambil() {
    const h = host.trim()
    if (!h) {
      const contoh = domain ? 'Isi nama domain terlebih dahulu, misalnya tokoanda.id.' : 'Isi host terlebih dahulu, misalnya tokoanda.id.'
      setGalat((g) => ({ ...g, host: contoh }))
      return
    }
    const n = ++permintaan.current
    hapusGalat('host')
    setHasil({ status: 'memuat', host: h, jenis: domain ? 'domain' : 'sertifikat' })
    try {
      if (domain) {
        const p = await api.pratinjauDomain(h)
        if (n !== permintaan.current) return
        // A registry answer switches a domain back from a typed date to the registry.
        setHasil(dariDomain(p))
        setTanggalSendiri(false)
        setKedaluwarsa('')
        hapusGalat('kedaluwarsa')
        return
      }
      const p = await api.pratinjau(h)
      if (n !== permintaan.current) return
      setHasil(dariPratinjau(p))
    } catch (err) {
      if (n !== permintaan.current) return
      setHasil({ status: 'kosong' })
      const pesan = err instanceof ApiError && err.field.host ? err.field.host : pesanGalat(err)
      setGalat((g) => ({ ...g, host: pesan }))
    }
  }

  // Empty state: the host was typed already, so start reading at once.
  const mulai = useRef(false)
  useEffect(() => {
    if (awal?.ambil && !mulai.current) {
      mulai.current = true
      void ambil()
    }
  }, [])

  const simpan = useMutation({
    mutationFn: (input: ItemInput) => (item ? api.ubah(item.id, input) : api.buat(input)),
    onSuccess: (it) => onSaved(it, !item),
    onError: (err) => {
      if (err instanceof ApiError && Object.keys(err.field).length > 0) {
        const tanggalTampil = !pakaiHost || cadangan || domainManual
        const tampil = new Set(['nama', 'klien', 'biaya', 'jenis', ...(pakaiHost ? ['host'] : []), ...(tanggalTampil ? ['kedaluwarsa'] : [])])
        const lain = Object.entries(err.field).filter(([k]) => !tampil.has(k))
        setGalat(err.field)
        setGalatUmum(lain.length ? lain.map(([, v]) => v).join(' ') : null)
        if (err.field.host) setHasil({ status: 'kosong' })
      } else {
        setGalatUmum(pesanGalat(err))
      }
    },
  })

  const biayaAngka = biaya ? Number(biaya) : null
  const bisaSimpan =
    !simpan.isPending &&
    hasil.status !== 'memuat' &&
    (sertifikat
      ? host.trim() !== '' && (!galat.host || kedaluwarsa !== '')
      : dariRegistri
        ? host.trim() !== '' && !galat.host
        : namaTampil.trim() !== '' && kedaluwarsa !== '')

  function kirim() {
    if (!bisaSimpan) return
    setGalatUmum(null)
    const dasar = {
      ...(klienNya.id ? { klien_id: klienNya.id } : { klien: klienNya.nama.trim() }),
      biaya: biayaAngka,
      catatan: item?.catatan ?? '',
    }
    let input: ItemInput
    if (sertifikat && cadangan) {
      input = { ...dasar, jenis: 'lainnya', nama: namaDisentuh && nama.trim() ? nama.trim() : host.trim(), kedaluwarsa }
    } else if (sertifikat || dariRegistri) {
      // No date: the API reads it from the server or the registry and names the item after the host or domain.
      input = { ...dasar, jenis, host: host.trim(), nama: namaDisentuh ? nama.trim() : '' }
    } else if (domain) {
      input = { ...dasar, jenis, nama: namaTampil.trim(), kedaluwarsa }
      if (host.trim()) input.host = host.trim()
    } else {
      input = { ...dasar, jenis, nama: nama.trim(), kedaluwarsa }
      if (item?.host && item.jenis !== 'sertifikat') input.host = item.host
    }
    simpan.mutate(input)
  }

  const fokusTanggal = buka.mode === 'ubah' && buka.fokus === 'kedaluwarsa'
  const fokusHost = !fokusTanggal && (sertifikat || dariRegistri)
  const judul = item ? 'Ubah aset' : 'Tambah aset'

  const fieldTanggal = (hint: ReactNode) => (
    <div className="field">
      <label className="field__label" htmlFor={id('tgl')}>
        Tanggal berakhir
      </label>
      <label className="input-icon">
        <Icon name="calendar" />
        <input
          id={id('tgl')}
          type="date"
          className={kedaluwarsa ? 'control num has-value' : 'control num'}
          value={kedaluwarsa}
          min="2000-01-01"
          max="2100-12-31"
          data-autofocus={fokusTanggal ? '' : undefined}
          aria-invalid={!!galat.kedaluwarsa}
          aria-describedby={galat.kedaluwarsa ? id('tgl-e') : hint ? id('tgl-h') : undefined}
          onChange={(e) => {
            setKedaluwarsa(e.target.value)
            hapusGalat('kedaluwarsa')
          }}
        />
      </label>
      <FieldError id={id('tgl-e')} pesan={galat.kedaluwarsa} />
      {hint ? (
        <div className="field__hint" id={id('tgl-h')}>
          {hint}
        </div>
      ) : null}
    </div>
  )

  return (
    <>
      <div className="scrim" onClick={onClose} aria-hidden="true" />
      <div ref={ref} className="sheet" data-ketik-langsung={item ? undefined : ''} role="dialog" aria-modal="true" aria-labelledby={id('judul')} tabIndex={-1}>
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            kirim()
          }}
        >
          <div className="sheet__head">
            <h2 className="sheet__title" id={id('judul')}>
              {judul}
            </h2>
            <div className="sheet__head-aksi">
              {/* Close comes first in the DOM, so it keeps the initial focus; the reminder shows left of it. */}
              <button type="button" className="btn btn--ghost btn--icon" aria-label="Tutup" onClick={onClose}>
                <Icon name="x" />
              </button>
              {item ? (
                <a
                  className="btn btn--ghost btn--icon sheet__wa"
                  href={tautanPengingat(item)}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label="Kirim pengingat lewat WhatsApp"
                  title="Kirim pengingat lewat WhatsApp"
                >
                  <Icon name="message-circle" />
                </a>
              ) : null}
            </div>
          </div>
          <div className="sheet__body">
            {galatUmum ? (
              <div className="form-error" role="alert">
                <Icon name="alert" />
                <span>{galatUmum}</span>
              </div>
            ) : null}
            <div className="field">
              <span className="field__label" id={id('jenis')}>
                Jenis
              </span>
              <div className={galat.jenis ? 'seg is-full is-error' : 'seg is-full'} role="radiogroup" aria-labelledby={id('jenis')}>
                {urutanJenis.map((j) => (
                  <button
                    key={j}
                    type="button"
                    className="seg__item"
                    role="radio"
                    aria-checked={jenis === j}
                    aria-label={labelJenis[j]}
                    onClick={() => {
                      setJenis(j)
                      hapusGalat('jenis', 'kedaluwarsa', 'host', 'nama')
                      // A certificate answer means nothing for a domain, and the other way round.
                      if (hasil.status !== 'kosong' && hasil.jenis !== j) {
                        permintaan.current++
                        setHasil({ status: 'kosong' })
                      }
                    }}
                  >
                    {phone ? labelJenisPendek[j] : labelJenis[j]}
                  </button>
                ))}
              </div>
              <FieldError id={id('jenis-e')} pesan={galat.jenis} />
            </div>

            {pakaiHost ? (
              <>
                <div className="field">
                  <label className="field__label" htmlFor={id('host')}>
                    {domain ? 'Domain' : 'Host'}
                    {tanggalSendiri && !galat.host ? <span>Opsional</span> : null}
                  </label>
                  <div className="field-row">
                    <label className="input-icon">
                      <Icon name="globe" />
                      <input
                        id={id('host')}
                        className="control"
                        placeholder="misalnya tokokuemawar.id"
                        autoComplete="off"
                        autoCapitalize="off"
                        spellCheck={false}
                        inputMode="url"
                        value={host}
                        data-autofocus={fokusHost ? '' : undefined}
                        aria-invalid={!!galat.host}
                        aria-describedby={galat.host ? id('host-e') : undefined}
                        onChange={(e) => {
                          setHost(e.target.value)
                          hapusGalat('host')
                          if (hasil.status === 'ok' && hasil.host !== e.target.value.trim()) setHasil({ status: 'kosong' })
                        }}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            e.preventDefault()
                            void ambil()
                          }
                        }}
                      />
                    </label>
                    <button
                      type="button"
                      className="btn btn--secondary"
                      onClick={() => void ambil()}
                      aria-busy={hasil.status === 'memuat'}
                      disabled={hasil.status === 'memuat'}
                    >
                      <Icon
                        name={hasil.status === 'memuat' ? 'loader' : domain ? 'database' : 'server'}
                        className={hasil.status === 'memuat' ? 'spin' : undefined}
                      />
                      {domain ? 'Ambil dari registri' : 'Ambil dari server'}
                    </button>
                  </div>
                  <FieldError id={id('host-e')} pesan={galat.host} />
                </div>
                {cadangan ? (
                  fieldTanggal('Isi tanggal berakhirnya sendiri. Aset ini disimpan dengan jenis Lainnya dan tidak dicek ulang otomatis.')
                ) : domainManual ? (
                  fieldTanggal(
                    tanggalSendiri && !gagalBaca
                      ? 'Tanggal ini tidak dicek ulang otomatis. Untuk kembali ke registri, pilih Ambil dari registri.'
                      : 'Isi tanggal berakhirnya sendiri. Domain ini tidak dicek ulang otomatis.',
                  )
                ) : (
                  <div aria-live="polite">
                    <Result hasil={hasil} />
                  </div>
                )}
                {dariRegistri && hasil.status !== 'memuat' ? (
                  <div className="ganti-sumber">
                    {hasilDomain ? null : <span>Tanggal dibaca dari registri saat Anda menyimpan.</span>}
                    <button
                      type="button"
                      className="btn-teks"
                      onClick={() => {
                        setTanggalSendiri(true)
                        if (hasilDomain) setKedaluwarsa(hasilDomain.kedaluwarsa)
                      }}
                    >
                      Isi tanggal sendiri
                    </button>
                  </div>
                ) : null}
              </>
            ) : null}

            <div className="field-grid">
              <div className="field">
                <label className="field__label" htmlFor={id('nama')}>
                  Nama
                </label>
                <input
                  id={id('nama')}
                  className="control"
                  placeholder={sertifikat ? 'Sama dengan host' : domain ? 'Sama dengan domain' : 'Nama aset'}
                  autoComplete="off"
                  value={namaTampil}
                  data-autofocus={!fokusTanggal && !sertifikat && !dariRegistri ? '' : undefined}
                  aria-invalid={!!galat.nama}
                  aria-describedby={galat.nama ? id('nama-e') : undefined}
                  onChange={(e) => {
                    setNama(e.target.value)
                    setNamaDisentuh(true)
                    hapusGalat('nama')
                  }}
                />
                <FieldError id={id('nama-e')} pesan={galat.nama} />
              </div>
              <div className="field">
                <label className="field__label" htmlFor={id('klien')}>
                  Klien
                </label>
                <KlienPicker
                  id={id('klien')}
                  klien={klien}
                  value={klienNya}
                  invalid={!!galat.klien}
                  describedBy={galat.klien ? id('klien-e') : klienBaru ? id('klien-h') : undefined}
                  onChange={(v) => {
                    setKlien(v)
                    hapusGalat('klien')
                  }}
                />
                <FieldError id={id('klien-e')} pesan={galat.klien} />
                {klienBaru && !galat.klien ? (
                  <div className="field__hint" id={id('klien-h')}>
                    Klien baru, dibuat saat aset disimpan.
                  </div>
                ) : null}
              </div>
            </div>

            {pakaiHost ? null : fieldTanggal(null)}

            <div className="field">
              <label className="field__label" htmlFor={id('biaya')}>
                Biaya <span>Opsional</span>
              </label>
              <div className="input-affix">
                <span className="affix">Rp</span>
                <input
                  id={id('biaya')}
                  className="control num"
                  inputMode="numeric"
                  placeholder="0"
                  autoComplete="off"
                  value={biaya ? angka(Number(biaya)) : ''}
                  aria-invalid={!!galat.biaya}
                  aria-describedby={galat.biaya ? id('biaya-e') : undefined}
                  onChange={(e) => {
                    setBiaya(e.target.value.replace(/\D/g, '').replace(/^0+(?=\d)/, '').slice(0, 13))
                    hapusGalat('biaya')
                  }}
                />
              </div>
              <FieldError id={id('biaya-e')} pesan={galat.biaya} />
            </div>

            <div className="note">
              <Icon name="shield" />
              <span>Tulis nama dan tanggalnya saja, bukan nilai token atau kata sandi.</span>
            </div>
          </div>
          <div className="sheet__foot">
            {item ? (
              <button type="button" className="btn btn--ghost" onClick={() => onHapus(item)}>
                <Icon name="trash" />
                Hapus
              </button>
            ) : null}
            <button type="button" className="btn btn--secondary" onClick={onClose}>
              Batal
            </button>
            <button type="submit" className="btn btn--primary" disabled={!bisaSimpan} aria-busy={simpan.isPending}>
              {simpan.isPending ? <Icon name="loader" className="spin" /> : null}
              {simpan.isPending ? 'Menyimpan' : 'Simpan'}
            </button>
          </div>
        </form>
      </div>
    </>
  )
}
