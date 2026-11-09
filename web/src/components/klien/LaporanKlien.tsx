// The Laporan tab of a client's space: pick a month, write the summary the client reads, preview the document,
// then print it, share a read-only link or send a short message through WhatsApp.
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useId, useRef, useState } from 'react'
import { ApiError, KUNCI_KLIEN, klienApi, type Klien } from '../../lib/api'
import {
  bulanIni,
  daftarBulan,
  geserBulan,
  kunciLaporan,
  labelBulan,
  laporanApi,
  type Laporan,
} from '../../lib/api-laporan'
import { gabung, rupiahBiasa } from '../../lib/format'
import { Icon } from '../Icon'
import { IkonLaporan } from '../IkonLaporan'
import { CetakLaporan, LaporanDokumen, cetak, durasi } from '../LaporanDokumen'
import { pesanGalat } from '../Toasts'
import { BagikanLaporanDialog } from './BagikanLaporanDialog'
import { Pilihan } from '../Pilihan'

const BATAS_TEKS = 2000
const JEDA_SIMPAN = 1000

type StatusSimpan =
  | { k: 'tersimpan'; pada: Date | null }
  | { k: 'belum' }
  | { k: 'menyimpan' }
  | { k: 'galat'; pesan: string }

const fmtJam = new Intl.DateTimeFormat('id-ID', { hour: '2-digit', minute: '2-digit' })

/** The message for WhatsApp: the month, the client, a few facts and the link when there is one. */
export function teksWhatsApp(l: Laporan, url: string | null): string {
  const r = l.ringkasan
  const fakta: string[] = []
  if (r.pekerjaan > 0) fakta.push(`${r.pekerjaan} pekerjaan${r.menit > 0 ? ` (${durasi(r.menit)})` : ''}`)
  if (l.perpanjangan.length > 0) fakta.push(`${l.perpanjangan.length} perpanjangan`)
  const baris = [`Halo, berikut laporan bulanan ${l.klien.nama} untuk ${l.label_bulan}.`, '']
  const kapan = l.lampau ? `Pada ${l.label_bulan}` : 'Bulan ini'
  baris.push(fakta.length > 0 ? `${kapan} tercatat ${gabung(fakta)}.` : `${kapan} tidak ada pekerjaan yang dicatat.`)
  if (!l.lampau && l.jatuh_tempo.length > 0) {
    const biaya = l.jatuh_tempo.reduce((n, j) => n + (j.biaya ?? 0), 0)
    baris.push(
      `Dalam 60 hari ke depan ada ${l.jatuh_tempo.length} jatuh tempo${biaya > 0 ? ` dengan perkiraan biaya ${rupiahBiasa(biaya)}` : ''}.`,
    )
  }
  if (!l.lampau && r.per_status.lewat > 0) baris.push(`${r.per_status.lewat} aset sudah melewati tanggal berakhir dan perlu segera diperpanjang.`)
  baris.push('')
  baris.push(url ? `Laporan lengkap: ${url}` : 'Laporan lengkap saya kirim dalam bentuk PDF.')
  baris.push('', 'Terima kasih.')
  return baris.join('\n')
}

export function LaporanKlien({ klienId }: { klienId: string }) {
  // A fresh editor per client, so one client's unsaved summary never shows under another.
  return <Isi key={klienId} klienId={klienId} />
}

function Isi({ klienId }: { klienId: string }) {
  const uid = useId()
  const qc = useQueryClient()
  const kini = bulanIni()
  const [bulan, setBulan] = useState(kini)
  const [bagikan, setBagikan] = useState(false)

  const q = useQuery({
    queryKey: kunciLaporan.bulan(klienId, bulan),
    queryFn: () => laporanApi.ambil(klienId, bulan),
    placeholderData: (lama, qLama) => (qLama?.queryKey[1] === klienId ? lama : undefined),
  })
  const klien = useQuery({ queryKey: KUNCI_KLIEN, queryFn: klienApi.daftar })
  const k = klien.data?.find((x) => x.id === klienId) as (Klien & { laporan_aktif?: boolean }) | undefined
  const jalur = useQuery({
    queryKey: kunciLaporan.jalur(klienId),
    queryFn: () => null as string | null,
    enabled: false,
    staleTime: Infinity,
    gcTime: Infinity,
  }).data

  // ---------- the summary editor, saved per month ----------
  const [teks, setTeks] = useState('')
  const [dimuatUntuk, setDimuatUntuk] = useState<string | null>(null)
  const [status, setStatus] = useState<StatusSimpan>({ k: 'tersimpan', pada: null })
  const tersimpan = useRef<Record<string, string>>({})
  const urutan = useRef(0)
  const teksRef = useRef(teks)
  teksRef.current = teks
  const dimuatRef = useRef(dimuatUntuk)
  dimuatRef.current = dimuatUntuk

  const siapEdit = dimuatUntuk === bulan && !q.isPlaceholderData

  // Fill the editor once per month, from the server's text.
  useEffect(() => {
    if (!q.data || q.isPlaceholderData || q.data.bulan !== bulan || dimuatUntuk === bulan) return
    tersimpan.current[bulan] = q.data.catatan ?? ''
    setTeks(q.data.catatan ?? '')
    setDimuatUntuk(bulan)
    setStatus({ k: 'tersimpan', pada: null })
  }, [q.data, q.isPlaceholderData, bulan, dimuatUntuk])

  const simpan = useCallback(
    async (b: string, t: string) => {
      const id = ++urutan.current
      setStatus({ k: 'menyimpan' })
      try {
        await laporanApi.simpanCatatan(klienId, b, t)
        tersimpan.current[b] = t
        qc.setQueryData<Laporan>(kunciLaporan.bulan(klienId, b), (lama) => (lama ? { ...lama, catatan: t } : lama))
        if (id !== urutan.current) return
        setStatus(dimuatRef.current === b && teksRef.current !== t ? { k: 'belum' } : { k: 'tersimpan', pada: new Date() })
      } catch (err) {
        if (id !== urutan.current) return
        setStatus({ k: 'galat', pesan: pesanGalat(err) })
      }
    },
    [klienId, qc],
  )

  // Save a second after the last keystroke.
  useEffect(() => {
    if (!siapEdit || teks === (tersimpan.current[bulan] ?? '')) return
    setStatus((s) => (s.k === 'menyimpan' ? s : { k: 'belum' }))
    const t = window.setTimeout(() => void simpan(bulan, teks), JEDA_SIMPAN)
    return () => window.clearTimeout(t)
  }, [teks, siapEdit, bulan, simpan])

  /** Saves what is still unsaved right away (month change, blur, leaving the tab). */
  const simpanSekarang = useCallback(() => {
    const b = dimuatRef.current
    if (!b) return
    if (teksRef.current !== (tersimpan.current[b] ?? '')) void simpan(b, teksRef.current)
  }, [simpan])

  useEffect(() => () => simpanSekarang(), [simpanSekarang])

  const adaPerubahan = status.k === 'belum' || status.k === 'menyimpan'
  useEffect(() => {
    if (!adaPerubahan) return
    const tahan = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', tahan)
    return () => window.removeEventListener('beforeunload', tahan)
  }, [adaPerubahan])

  function pindahBulan(b: string) {
    if (b === bulan) return
    simpanSekarang()
    setDimuatUntuk(null)
    setBulan(b)
  }

  // ---------- month picker: the client's first month up to this month ----------
  const batas = useRef<{ pertama?: string; terakhir?: string }>({})
  if (q.data && !q.isPlaceholderData) batas.current = { pertama: q.data.bulan_pertama, terakhir: q.data.bulan_terakhir }
  const terakhir = batas.current.terakhir ?? kini
  const pertama = batas.current.pertama
  const pilihan = daftarBulan(terakhir, pertama)
  if (!pilihan.includes(bulan)) pilihan.push(bulan)
  const bisaMaju = bulan < terakhir
  const bisaMundur = !pertama || bulan > pertama
  const diLuarRentang = q.isError && q.error instanceof ApiError && q.error.status === 422

  const l = q.data
  const url = jalur ? `${window.location.origin}${jalur}?bulan=${bulan}` : null
  const tautanAktif = jalur != null || k?.laporan_aktif === true

  let teksStatus: React.ReactNode = null
  if (siapEdit) {
    if (status.k === 'menyimpan') {
      teksStatus = (
        <span className="lk__status">
          <Icon name="loader" className="spin" />
          Menyimpan
        </span>
      )
    } else if (status.k === 'belum') {
      teksStatus = <span className="lk__status">Belum tersimpan</span>
    } else if (status.k === 'galat') {
      teksStatus = (
        <span className="lk__status is-galat">
          Gagal menyimpan.{' '}
          <button type="button" className="btn-teks is-danger" onClick={() => void simpan(bulan, teks)}>
            Coba lagi
          </button>
        </span>
      )
    } else if (status.pada) {
      teksStatus = (
        <span className="lk__status is-ok">
          <Icon name="tick" />
          Tersimpan {fmtJam.format(status.pada).replace(':', '.')}
        </span>
      )
    } else {
      teksStatus = <span className="lk__status">Tersimpan otomatis saat Anda mengetik</span>
    }
  }

  const tombolSiap = !!l && !q.isPlaceholderData

  return (
    <div className="lk">
      <div className="lk__alat">
        <div className="lk__bulan" role="group" aria-label="Bulan laporan">
          <button
            type="button"
            className="btn btn--secondary btn--icon"
            aria-label={bisaMundur ? `Bulan sebelumnya, ${labelBulan(geserBulan(bulan, -1))}` : 'Bulan sebelumnya'}
            onClick={() => pindahBulan(geserBulan(bulan, -1))}
            disabled={!bisaMundur}
          >
            <IkonLaporan name="chevron-left" />
          </button>
          <Pilihan
            id={`${uid}-bulan`}
            label="Bulan laporan"
            className="laporan-bulan__pilih"
            nilai={bulan}
            opsi={pilihan.map((b) => ({ nilai: b, label: labelBulan(b) }))}
            onPilih={pindahBulan}
          />
          <button
            type="button"
            className="btn btn--secondary btn--icon"
            aria-label={bisaMaju ? `Bulan berikutnya, ${labelBulan(geserBulan(bulan, 1))}` : 'Bulan berikutnya'}
            onClick={() => pindahBulan(geserBulan(bulan, 1))}
            disabled={!bisaMaju}
          >
            <IkonLaporan name="chevron-right" />
          </button>
        </div>
        <div className="lk__aksi">
          <button type="button" className="btn btn--secondary" aria-haspopup="dialog" onClick={() => setBagikan(true)}>
            <Icon name="link" />
            Bagikan tautan
          </button>
          {tombolSiap ? (
            <a
              className="btn btn--secondary"
              href={`https://wa.me/?text=${encodeURIComponent(teksWhatsApp(l, url))}`}
              target="_blank"
              rel="noopener noreferrer"
            >
              <IkonLaporan name="message" />
              Kirim lewat WhatsApp
            </a>
          ) : (
            <button type="button" className="btn btn--secondary" disabled>
              <IkonLaporan name="message" />
              Kirim lewat WhatsApp
            </button>
          )}
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => {
              if (l) cetak(l)
            }}
            disabled={!tombolSiap}
          >
            <IkonLaporan name="printer" />
            Cetak / Simpan PDF
          </button>
        </div>
      </div>

      <div className="lk__isi">
        <div className="lk__pratinjau" aria-busy={q.isFetching}>
          {diLuarRentang && !l ? (
            <div className="lk__galat" role="alert">
              <b>Laporan untuk {labelBulan(bulan)} tidak tersedia.</b>
              <span>{(q.error as ApiError).field.bulan ?? 'Pilih bulan sejak klien ini ditambahkan.'}</span>
              <button type="button" className="btn btn--secondary" onClick={() => pindahBulan(kini)}>
                Lihat laporan {labelBulan(kini)}
              </button>
            </div>
          ) : q.isError && !l ? (
            <div className="lk__galat" role="alert">
              <b>Laporan tidak dapat dimuat.</b>
              <span>{pesanGalat(q.error)}</span>
              <button type="button" className="btn btn--secondary" onClick={() => void q.refetch()}>
                <Icon name="refresh" />
                Coba lagi
              </button>
            </div>
          ) : l ? (
            <div style={q.isPlaceholderData ? { opacity: 0.55, transition: 'opacity 120ms' } : undefined}>
              <LaporanDokumen laporan={l} catatan={siapEdit ? teks : undefined} varian="pemilik" tingkat={2} />
            </div>
          ) : (
            <KerangkaLaporan />
          )}
        </div>

        <aside className="lk__panel" aria-label="Pengaturan laporan">
          <div className="lk__kartu">
            <label className="lk__judul" htmlFor={`${uid}-teks`}>
              Ringkasan untuk klien
            </label>
            <p className="lk__teks" id={`${uid}-teks-hint`}>
              Tampil di bagian atas laporan {labelBulan(bulan)}. Tulis apa yang dikerjakan dan saran untuk bulan depan.
            </p>
            {siapEdit ? (
              <textarea
                id={`${uid}-teks`}
                className="control lk__editor"
                value={teks}
                maxLength={BATAS_TEKS}
                aria-describedby={`${uid}-teks-hint ${uid}-teks-status`}
                placeholder="Contoh: Sertifikat dan domain diperpanjang tepat waktu. Kami menyarankan pembaruan PHP ke versi 8.3 pada bulan Desember."
                onChange={(e) => setTeks(e.target.value)}
                onBlur={simpanSekarang}
              />
            ) : (
              <span className="sk" style={{ width: '100%', height: 'var(--lk-editor-h)' }} aria-hidden="true" />
            )}
            <div className="lk__simpan" id={`${uid}-teks-status`} aria-live="polite">
              {teksStatus}
              {siapEdit ? (
                <span className="lk__hitung num">
                  {teks.length.toLocaleString('id-ID')}/{BATAS_TEKS.toLocaleString('id-ID')}
                </span>
              ) : null}
            </div>
          </div>
          <div className="lk__kartu">
            <span className="lk__judul">Tautan baca-saja</span>
            <span className="lk__tautan">
              <span className={tautanAktif ? 'dot st-aman' : 'dot is-off'} aria-hidden="true" />
              {tautanAktif ? 'Aktif. Klien dapat membuka laporan tanpa masuk.' : 'Belum dibuat.'}
            </span>
            <p className="lk__teks">
              Klien dapat membuka laporan bulan mana pun lewat tautan ini. Catatan internal tidak ikut ditampilkan.
            </p>
          </div>
        </aside>
      </div>

      {l && !q.isPlaceholderData ? <CetakLaporan laporan={l} catatan={siapEdit ? teks : undefined} /> : null}
      {bagikan ? (
        <BagikanLaporanDialog
          klienId={klienId}
          bulan={bulan}
          aktif={k ? (k.laporan_aktif ?? false) : klien.isError ? false : undefined}
          onClose={() => setBagikan(false)}
        />
      ) : null}
    </div>
  )
}

function KerangkaLaporan() {
  return (
    <div className="lap" aria-busy="true" aria-label="Memuat laporan">
      <span className="sk" style={{ width: '100%', height: 12 }} />
      <span className="sk" style={{ width: '55%', height: 32, marginTop: 40 }} />
      <span className="sk" style={{ width: '30%', height: 20, marginTop: 12 }} />
      <span className="sk" style={{ width: '100%', height: 88, marginTop: 40 }} />
      {[0, 1, 2, 3, 4].map((i) => (
        <span key={i} className="sk" style={{ width: '100%', height: 16, marginTop: 16 }} />
      ))}
    </div>
  )
}
