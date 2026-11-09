// Tab "Aktivitas" of a client's space: a composer for internal notes and work entries, then the feed, newest day
// first. Entries for assets, checks and tasks come from the API.
import { useInfiniteQuery, useMutation } from '@tanstack/react-query'
import { useId, useMemo, useState } from 'react'
import { ApiError } from '../../lib/api'
import {
  aktivitasApi,
  durasi,
  kunciAktivitas,
  useSegarkan,
  type Aktivitas,
  type AktivitasInput,
  type JenisAktivitas,
} from '../../lib/api-tugas'
import { hariIniIso, keTanggal, tanggalPanjang } from '../../lib/format'
import { ConfirmDialog } from '../ConfirmDialog'
import { Icon } from '../Icon'
import { pesanGalat, useToast } from '../Toasts'
import { Ikon, ikonAktivitas } from './ikon'
import '../../styles/tugas.css'

type JenisTulis = 'catatan' | 'pekerjaan'

const labelJenis: Record<JenisAktivitas, string> = {
  catatan: 'Catatan internal',
  pekerjaan: 'Pekerjaan',
  aset_baru: 'Aset',
  aset_diperpanjang: 'Aset',
  aset_dihapus: 'Aset',
  cek_diperbarui: 'Pemeriksaan',
  cek_gagal: 'Pemeriksaan',
  tugas_baru: 'Tugas',
  tugas_selesai: 'Tugas',
  tugas_dihapus: 'Tugas',
  laporan_dibagikan: 'Laporan',
}

const nada: Partial<Record<JenisAktivitas, string>> = {
  pekerjaan: 'is-kerja',
  tugas_selesai: 'is-kerja',
  cek_gagal: 'is-galat',
  aset_diperpanjang: 'is-ok',
  cek_diperbarui: 'is-ok',
}

const isoLokal = (d: Date) => hariIniIso(d)

function judulHari(iso: string, hariIni: string): string {
  if (iso === hariIni) return 'Hari ini'
  const k = keTanggal(hariIni)
  if (iso === isoLokal(new Date(k.getFullYear(), k.getMonth(), k.getDate() - 1))) return 'Kemarin'
  return tanggalPanjang(iso)
}

const fmtJam = new Intl.DateTimeFormat('id-ID', { hour: '2-digit', minute: '2-digit' })
const fmtTglPendek = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short' })

/** "09.42" when the entry was written on its own day, "dicatat 12 Nov" for work logged later. */
function waktu(a: Aktivitas): string {
  const d = new Date(a.dibuat)
  if (isoLokal(d) === a.tanggal) return fmtJam.format(d)
  return `dicatat ${fmtTglPendek.format(d)}`
}

export function AktivitasKlien({ klienId }: { klienId: string }) {
  const toast = useToast()
  const segarkan = useSegarkan()
  const [hapus, setHapus] = useState<Aktivitas | null>(null)

  const q = useInfiniteQuery({
    queryKey: kunciAktivitas.klien(klienId),
    queryFn: ({ pageParam }) => aktivitasApi.daftar({ klienId, sebelum: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => (last.lagi && last.aktivitas.length > 0 ? last.aktivitas[last.aktivitas.length - 1].id : undefined),
  })

  const hariIni = hariIniIso()
  const kelompok = useMemo(() => {
    const semua = (q.data?.pages ?? []).flatMap((p) => p.aktivitas)
    const unik = [...new Map(semua.map((a) => [a.id, a])).values()]
    unik.sort((a, b) => b.tanggal.localeCompare(a.tanggal) || b.dibuat.localeCompare(a.dibuat))
    const hasil: { tanggal: string; isi: Aktivitas[]; menit: number }[] = []
    for (const a of unik) {
      let g = hasil[hasil.length - 1]
      if (!g || g.tanggal !== a.tanggal) {
        g = { tanggal: a.tanggal, isi: [], menit: 0 }
        hasil.push(g)
      }
      g.isi.push(a)
      if (a.durasi_menit && (a.jenis === 'pekerjaan' || a.jenis === 'tugas_selesai')) g.menit += a.durasi_menit
    }
    return hasil
  }, [q.data])

  const hapusM = useMutation({
    mutationFn: (a: Aktivitas) => aktivitasApi.hapus(a.id),
    onSuccess: (_, a) => {
      setHapus(null)
      segarkan()
      toast.ok(a.jenis === 'pekerjaan' ? 'Pekerjaan dihapus' : 'Catatan dihapus')
    },
    onError: (err) => toast.dariGalat('Entri tidak terhapus', err),
  })

  let isi
  if (q.isPending) {
    isi = (
      <div className="ak-feed" aria-busy="true" aria-label="Memuat aktivitas">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="ak-item">
            <span className="sk" style={{ width: 32, height: 32, borderRadius: 999 }} />
            <span className="sk" style={{ width: `${70 - i * 10}%`, height: 16, marginTop: 8 }} />
          </div>
        ))}
      </div>
    )
  } else if (q.isError) {
    isi = (
      <div className="tg-galat" role="alert">
        <b>Aktivitas tidak dapat dimuat.</b>
        <span>{pesanGalat(q.error)}</span>
        <button type="button" className="btn btn--secondary" onClick={() => void q.refetch()}>
          <Icon name="refresh" />
          Coba lagi
        </button>
      </div>
    )
  } else if (kelompok.length === 0) {
    isi = (
      <div className="ak-empty">
        <b>Belum ada aktivitas.</b>
        <span>
          Tulis catatan atau catat pekerjaan di atas. Perubahan aset dan tugas klien ini juga tercatat di sini.
        </span>
      </div>
    )
  } else {
    isi = (
      <div className="ak-feed">
        {kelompok.map((g) => (
          <section key={g.tanggal} className="ak-day" aria-label={judulHari(g.tanggal, hariIni)}>
            <h3 className="ak-day__head">
              <span>{judulHari(g.tanggal, hariIni)}</span>
              {g.menit > 0 ? <span className="ak-day__sum num">{durasi(g.menit)} kerja</span> : null}
            </h3>
            <ol className="ak-list">
              {g.isi.map((a) => (
                <Entri key={a.id} a={a} onHapus={() => setHapus(a)} />
              ))}
            </ol>
          </section>
        ))}
        {q.hasNextPage ? (
          <div className="ak-more">
            <button
              type="button"
              className="btn btn--secondary"
              onClick={() => void q.fetchNextPage()}
              disabled={q.isFetchingNextPage}
              aria-busy={q.isFetchingNextPage}
            >
              {q.isFetchingNextPage ? <Icon name="loader" className="spin" /> : null}
              {q.isFetchingNextPage ? 'Memuat' : 'Muat lagi'}
            </button>
          </div>
        ) : null}
        {q.isFetchNextPageError ? (
          <p className="ak-more__err" role="alert">
            Entri lama tidak dapat dimuat. Coba lagi.
          </p>
        ) : null}
      </div>
    )
  }

  return (
    <section className="ak" aria-label="Aktivitas">
      <Penulis
        klienId={klienId}
        onSaved={(a) => {
          segarkan()
          toast.ok(a.jenis === 'pekerjaan' ? 'Pekerjaan dicatat' : 'Catatan disimpan')
        }}
      />
      {isi}
      {hapus ? (
        <ConfirmDialog
          judul={hapus.jenis === 'pekerjaan' ? 'Hapus pekerjaan?' : 'Hapus catatan?'}
          tombol="Hapus"
          tombolSedang="Menghapus"
          sedang={hapusM.isPending}
          onBatal={() => setHapus(null)}
          onYa={() => hapusM.mutate(hapus)}
        >
          {hapus.jenis === 'pekerjaan'
            ? 'Entri ini hilang dari aktivitas dan dari laporan bulanan klien.'
            : 'Catatan ini hilang dari aktivitas klien.'}
        </ConfirmDialog>
      ) : null}
    </section>
  )
}

function Entri({ a, onHapus }: { a: Aktivitas; onHapus: () => void }) {
  const bisaHapus = a.jenis === 'catatan' || a.jenis === 'pekerjaan'
  // A finished task keeps the note written in "Tandai selesai" in data.catatan.
  const catatan = typeof a.data?.catatan === 'string' && a.data.catatan !== '' ? a.data.catatan : null
  return (
    <li className={`ak-item ${nada[a.jenis] ?? ''}`}>
      <span className="ak-item__icon" aria-hidden="true">
        <Ikon name={ikonAktivitas[a.jenis]} />
      </span>
      <div className="ak-item__body">
        <p className="ak-item__text">{a.teks}</p>
        {catatan ? <p className="ak-item__note">{catatan}</p> : null}
        <p className="ak-item__meta num">
          <span className={a.jenis === 'catatan' ? 'ak-tag is-internal' : 'ak-tag'}>{labelJenis[a.jenis]}</span>
          <span>{waktu(a)}</span>
          {a.durasi_menit ? (
            <span className="ak-item__dur">
              <Ikon name="clock" />
              {durasi(a.durasi_menit)}
            </span>
          ) : null}
        </p>
      </div>
      {bisaHapus ? (
        <button
          type="button"
          className="btn btn--ghost is-sm btn--icon ak-item__del"
          aria-label={a.jenis === 'pekerjaan' ? 'Hapus pekerjaan' : 'Hapus catatan'}
          title="Hapus"
          onClick={onHapus}
        >
          <Icon name="trash" />
        </button>
      ) : null}
    </li>
  )
}

/** Moves focus to the first field marked invalid once it renders. */
const fokusGalat = () =>
  requestAnimationFrame(() => document.querySelector<HTMLElement>('.ak-composer [aria-invalid="true"]')?.focus())

const hanyaAngka = (s: string) => s.replace(/\D/g, '').replace(/^0+(?=\d)/, '').slice(0, 4)

function Penulis({ klienId, onSaved }: { klienId: string; onSaved: (a: Aktivitas) => void }) {
  const uid = useId()
  const id = (s: string) => `${uid}-${s}`
  const hariIni = hariIniIso()
  const [jenis, setJenis] = useState<JenisTulis>('pekerjaan')
  const [teks, setTeks] = useState('')
  const [tgl, setTgl] = useState(hariIni)
  const [menit, setMenit] = useState('')
  const [galat, setGalat] = useState<Record<string, string>>({})
  const [galatUmum, setGalatUmum] = useState<string | null>(null)

  const simpan = useMutation({
    mutationFn: (input: AktivitasInput) => aktivitasApi.buat(input),
    onSuccess: (a) => {
      setTeks('')
      setMenit('')
      setGalat({})
      onSaved(a)
    },
    onError: (err) => {
      if (err instanceof ApiError && Object.keys(err.field).length > 0) setGalat(err.field)
      else setGalatUmum(pesanGalat(err))
    },
  })

  function kirim() {
    if (simpan.isPending) return
    const g: Record<string, string> = {}
    if (teks.trim() === '') g.teks = jenis === 'pekerjaan' ? 'Tulis pekerjaan yang dilakukan.' : 'Tulis catatannya.'
    if (!tgl) g.tanggal = 'Pilih tanggal.'
    else if (tgl > hariIni) g.tanggal = 'Tanggal tidak boleh setelah hari ini.'
    if (jenis === 'pekerjaan' && menit !== '' && !(Number(menit) >= 1 && Number(menit) <= 1440))
      g.durasi_menit = 'Isi 1 sampai 1440 menit.'
    if (Object.keys(g).length > 0) {
      setGalat(g)
      fokusGalat()
      return
    }
    setGalatUmum(null)
    simpan.mutate({
      klien_id: klienId,
      jenis,
      teks: teks.trim(),
      tanggal: tgl,
      durasi_menit: jenis === 'pekerjaan' && menit !== '' ? Number(menit) : undefined,
    })
  }

  const pilihan: { v: JenisTulis; label: string }[] = [
    { v: 'catatan', label: 'Catatan' },
    { v: 'pekerjaan', label: 'Pekerjaan' },
  ]

  return (
    <form
      className="ak-composer"
      noValidate
      aria-label="Tulis aktivitas"
      onSubmit={(e) => {
        e.preventDefault()
        kirim()
      }}
    >
      <div className="ak-composer__top">
        <div className="seg is-sm" role="radiogroup" aria-label="Jenis entri">
          {pilihan.map((p) => (
            <button
              key={p.v}
              type="button"
              className="seg__item"
              role="radio"
              aria-checked={jenis === p.v}
              tabIndex={jenis === p.v ? 0 : -1}
              onClick={() => setJenis(p.v)}
              onKeyDown={(e) => {
                if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
                e.preventDefault()
                const lain = pilihan.find((x) => x.v !== p.v)!
                setJenis(lain.v)
                const sib = (e.currentTarget.parentElement?.querySelector(`[data-v="${lain.v}"]`) as HTMLElement | null)
                sib?.focus()
              }}
              data-v={p.v}
            >
              {p.label}
            </button>
          ))}
        </div>
        <p className="ak-composer__hint" id={id('hint')}>
          {jenis === 'pekerjaan' ? (
            <>
              <Icon name="info" />
              Masuk ke laporan bulanan klien.
            </>
          ) : (
            <>
              <Icon name="lock" />
              Hanya untuk Anda. Tidak masuk laporan.
            </>
          )}
        </p>
      </div>
      {galatUmum ? (
        <div className="form-error" role="alert">
          <Icon name="alert" />
          <span>{galatUmum}</span>
        </div>
      ) : null}
      <label className="sr-only" htmlFor={id('teks')}>
        {jenis === 'pekerjaan' ? 'Pekerjaan yang dilakukan' : 'Catatan'}
      </label>
      <textarea
        id={id('teks')}
        className="control tg-textarea ak-composer__text"
        placeholder={jenis === 'pekerjaan' ? 'Apa yang Anda kerjakan untuk klien ini?' : 'Catatan untuk Anda sendiri'}
        rows={2}
        maxLength={2000}
        value={teks}
        aria-invalid={!!galat.teks}
        aria-describedby={galat.teks ? id('teks-e') : id('hint')}
        onChange={(e) => {
          setTeks(e.target.value)
          if (galat.teks) setGalat((g) => ({ ...g, teks: '' }))
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
            e.preventDefault()
            kirim()
          }
        }}
      />
      {galat.teks ? (
        <div className="field__error" id={id('teks-e')}>
          <Icon name="alert" />
          <span>{galat.teks}</span>
        </div>
      ) : null}
      <div className="ak-composer__bar">
        <label className="input-icon ak-composer__date">
          <Icon name="calendar" />
          <span className="sr-only">Tanggal</span>
          <input
            type="date"
            className={tgl ? 'control is-sm num has-value' : 'control is-sm num'}
            value={tgl}
            min="2000-01-01"
            max={hariIni}
            aria-invalid={!!galat.tanggal}
            aria-describedby={galat.tanggal ? id('tgl-e') : undefined}
            onChange={(e) => {
              setTgl(e.target.value)
              setGalat((g) => ({ ...g, tanggal: '' }))
            }}
          />
        </label>
        {jenis === 'pekerjaan' ? (
          <div className="tg-suffix ak-composer__min">
            <input
              className="control is-sm num"
              inputMode="numeric"
              placeholder="Durasi"
              aria-label="Durasi dalam menit, opsional"
              autoComplete="off"
              value={menit}
              aria-invalid={!!galat.durasi_menit}
              aria-describedby={galat.durasi_menit ? id('menit-e') : undefined}
              onChange={(e) => {
                setMenit(hanyaAngka(e.target.value))
                setGalat((g) => ({ ...g, durasi_menit: '' }))
              }}
            />
            <span className="tg-suffix__u" aria-hidden="true">
              menit
            </span>
          </div>
        ) : null}
        <button type="submit" className="btn btn--primary is-sm ak-composer__send" disabled={simpan.isPending} aria-busy={simpan.isPending}>
          {simpan.isPending ? <Icon name="loader" className="spin" /> : null}
          {simpan.isPending ? 'Menyimpan' : jenis === 'pekerjaan' ? 'Catat pekerjaan' : 'Simpan catatan'}
        </button>
      </div>
      {galat.tanggal || galat.durasi_menit ? (
        <div className="field__error" id={galat.tanggal ? id('tgl-e') : id('menit-e')}>
          <Icon name="alert" />
          <span>{galat.tanggal || galat.durasi_menit}</span>
        </div>
      ) : null}
    </form>
  )
}
