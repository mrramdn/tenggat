import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useState } from 'react'
import type { Akun } from './components/Akun'
import { AssetSheet, type ModeSheet } from './components/AssetSheet'
import { ConfirmDialog } from './components/ConfirmDialog'
import { Desktop, type Muat } from './components/Desktop'
import { Icon } from './components/Icon'
import { Beranda } from './components/Beranda'
import { HalamanLaporan } from './components/HalamanLaporan'
import { KlienDialog } from './components/KlienDialog'
import { LayarMasuk } from './components/LayarMasuk'
import { Phone } from './components/Phone'
import { TidakDitemukan } from './components/TidakDitemukan'
import { pesanGalat, useToast } from './components/Toasts'
import type { Aksi, BukaTambah } from './components/types'
import { adaModal } from './hooks/useModal'
import { useMediaQuery } from './hooks/useMediaQuery'
import { KUNCI_KLIEN, api, auth, klienApi, saatSesiBerakhir, type Item, type Klien, type Pengguna } from './lib/api'
import { saringAwal, type Saring } from './lib/derive'
import { tanggal } from './lib/format'
import { jalurKlien, jalurMasuk, perluMasuk, pergi, tujuanLanjut, useJudulHalaman, useRute, type Rute } from './lib/rute'

const KUNCI = ['item'] as const
const KUNCI_SAYA = ['saya'] as const

function sedangMengetik(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false
  return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName)
}

/** Sends the browser to another path as soon as it renders (redirects), showing the blank boot screen meanwhile. */
function Alih({ ke }: { ke: string }) {
  useEffect(() => pergi(ke, { ganti: true }), [ke])
  return <div className="boot" aria-busy="true" aria-label="Memuat" />
}

/**
 * Routes and the auth gate. Public pages (/, /laporan/:token, not found) render without a session; /masuk and
 * /daftar send a signed-in user on to the app; everything under /app asks the API who is signed in (GET /api/saya)
 * and goes to /masuk?lanjut=<path> without a session.
 */
export function App() {
  const rute = useRute()
  const qc = useQueryClient()
  const saya = useQuery({ queryKey: KUNCI_SAYA, queryFn: auth.saya, staleTime: Infinity })
  const [pemberitahuan, setPemberitahuan] = useState<string | null>(null)

  // Signed out (sign out, account deleted or session expired): drop every cached answer of the previous user and
  // go to the given path (the front page, or the sign-in page that comes back here).
  const keluarKe = useCallback(
    (tujuan: string, pesan: string | null) => {
      pergi(tujuan, { ganti: true })
      qc.setQueryData(KUNCI_SAYA, null)
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== KUNCI_SAYA[0] })
      qc.getMutationCache().clear()
      setPemberitahuan(pesan)
    },
    [qc],
  )

  // Any 401 from a data request while signed in means the session ended.
  useEffect(() => {
    saatSesiBerakhir(() => {
      if (!qc.getQueryData(KUNCI_SAYA)) return
      keluarKe(jalurMasuk(), 'Sesi Anda berakhir. Silakan masuk lagi.')
    })
    return () => saatSesiBerakhir(null)
  }, [qc, keluarKe])

  const toast = useToast()
  const masuk = useCallback(
    (p: Pengguna, baru: boolean) => {
      qc.removeQueries({ predicate: (q) => q.queryKey[0] !== KUNCI_SAYA[0] })
      pergi(tujuanLanjut(), { ganti: true })
      qc.setQueryData(KUNCI_SAYA, p)
      setPemberitahuan(null)
      if (baru) toast.ok('Akun dibuat.', `Selamat datang, ${p.nama}.`)
    },
    [qc, toast],
  )

  // The sign-in pages and the not-found page; the app (Dasbor) and the public pages set their own title.
  useJudulHalaman(
    rute.nama === 'masuk' ? 'Masuk' : rute.nama === 'daftar' ? 'Buat akun' : rute.nama === 'tidak-ditemukan' ? 'Halaman tidak ditemukan' : undefined,
  )

  if (rute.nama === 'laporan-publik') return <HalamanLaporan token={rute.token} />
  if (rute.nama === 'beranda') return <Beranda sudahMasuk={!!saya.data} />
  if (rute.nama === 'tidak-ditemukan') return <TidakDitemukan sudahMasuk={!!saya.data} />

  if (saya.isPending) return <div className="boot" aria-busy="true" aria-label="Memuat" />
  if (saya.isError) {
    return (
      <div className="boot">
        <div className="state" role="alert">
          <div className="wordmark">tenggat</div>
          <h1 className="display">Tenggat tidak dapat dihubungi.</h1>
          <p>{pesanGalat(saya.error)}</p>
          <button type="button" className="btn btn--primary" onClick={() => void saya.refetch()}>
            <Icon name="refresh" />
            Coba lagi
          </button>
        </div>
      </div>
    )
  }
  if (rute.nama === 'masuk' || rute.nama === 'daftar') {
    if (saya.data) return <Alih ke={tujuanLanjut()} />
    return <LayarMasuk mode={rute.nama} pemberitahuan={pemberitahuan} onMasuk={masuk} />
  }
  if (perluMasuk(rute) && !saya.data) return <Alih ke={jalurMasuk()} />
  if (!saya.data) return null
  return <Dasbor key={saya.data.id} pengguna={saya.data} rute={rute} onKeluar={keluarKe} />
}

function Dasbor({ pengguna, rute, onKeluar }: { pengguna: Pengguna; rute: Rute; onKeluar: (tujuan: string, pesan: string | null) => void }) {
  const phone = useMediaQuery('(max-width: 1023px)')
  const qc = useQueryClient()
  const toast = useToast()
  const q = useQuery({ queryKey: KUNCI, queryFn: () => api.daftar(), refetchInterval: 60_000 })
  const kq = useQuery({ queryKey: KUNCI_KLIEN, queryFn: klienApi.daftar, refetchInterval: 60_000 })

  const [saring, setSaring] = useState<Saring>(saringAwal)
  const [sheet, setSheet] = useState<ModeSheet | null>(null)
  const [hapus, setHapus] = useState<Item | null>(null)
  const [sedangCek, setSedangCek] = useState<ReadonlySet<string>>(new Set())
  const [hapusAkun, setHapusAkun] = useState(false)
  // undefined: closed; null: adding a client; a client: editing it
  const [dialogKlien, setDialogKlien] = useState<Klien | null | undefined>(undefined)
  const [hapusKlien, setHapusKlien] = useState<Klien | null>(null)

  // An asset write changes the clients' figures too, and the API logs it as activity (tasks follow the same
  // clients), so every list that can show it is refreshed.
  const segarkan = useCallback(
    () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: KUNCI }),
        qc.invalidateQueries({ queryKey: KUNCI_KLIEN }),
        qc.invalidateQueries({ queryKey: ['aktivitas'] }),
        qc.invalidateQueries({ queryKey: ['tugas'] }),
      ]),
    [qc],
  )

  // A new page starts without the search and the type, status and source filters of the previous one.
  const kunciRute = rute.nama === 'klien' ? `klien/${rute.id}` : rute.nama
  useEffect(() => {
    setSaring((s) => ({ ...saringAwal, klien: s.klien, naik: s.naik }))
  }, [kunciRute])
  const klienRute = rute.nama === 'klien' ? rute.id : null

  // Dev only: ?buka=tambah[&host=...]|ubah&id=... opens the sheet, for screenshots.
  useEffect(() => {
    if (!import.meta.env.DEV) return
    const p = new URLSearchParams(location.search)
    if (p.get('buka') === 'tambah') {
      const host = p.get('host') ?? undefined
      setSheet({ mode: 'tambah', awal: { jenis: 'sertifikat', host, ambil: !!host } })
    }
  }, [])
  useEffect(() => {
    if (!import.meta.env.DEV || !q.data) return
    const p = new URLSearchParams(location.search)
    const id = p.get('id')
    if (p.get('buka') === 'ubah' && id) {
      const it = q.data.find((x) => x.id === id)
      if (it) setSheet((s) => s ?? { mode: 'ubah', item: it, fokus: p.get('fokus') === 'tanggal' ? 'kedaluwarsa' : undefined })
    }
  }, [q.data])

  const hapusM = useMutation({
    mutationFn: (it: Item) => api.hapus(it.id),
    onSuccess: (_, it) => {
      toast.ok('Aset dihapus.', it.nama)
      setHapus(null)
      setSheet(null)
      void segarkan()
    },
    onError: (err) => toast.dariGalat('Aset tidak dapat dihapus.', err),
  })

  const hapusKlienM = useMutation({
    mutationFn: (k: Klien) => klienApi.hapus(k.id),
    onSuccess: (_, k) => {
      toast.ok('Klien dihapus.', k.nama)
      setHapusKlien(null)
      setSaring((s) => (s.klien === k.id ? { ...s, klien: null } : s))
      if (klienRute === k.id) pergi('/app', { ganti: true })
      void segarkan()
    },
    onError: (err) => toast.dariGalat('Klien tidak dapat dihapus.', err),
  })

  const keluarM = useMutation({
    mutationFn: () => auth.keluar(),
    onSuccess: () => {
      onKeluar('/', null)
      toast.ok('Anda sudah keluar.')
    },
    onError: (err) => toast.dariGalat('Tidak dapat keluar.', err),
  })

  const hapusAkunM = useMutation({
    mutationFn: () => auth.hapusAkun(),
    onSuccess: () => {
      onKeluar('/', null)
      toast.ok('Akun dihapus.', 'Akun dan semua data Anda sudah dihapus.')
    },
    onError: (err) => toast.dariGalat('Akun tidak dapat dihapus.', err),
  })

  const akun = useMemo<Akun>(
    () => ({
      pengguna,
      keluar: () => keluarM.mutate(),
      hapus: () => setHapusAkun(true),
      sedangKeluar: keluarM.isPending,
    }),
    [pengguna, keluarM.mutate, keluarM.isPending],
  )

  const cekUlang = useCallback(
    async (it: Item) => {
      setSedangCek((s) => new Set(s).add(it.id))
      try {
        const r = await api.cekUlang(it.id)
        if (r.galat_cek) toast.galat('Cek gagal.', `${r.nama}: ${r.galat_cek}`)
        else toast.ok(r.jenis === 'sertifikat' ? 'Sertifikat dicek ulang.' : 'Domain dicek ulang.', `${r.nama} berakhir ${tanggal(r.kedaluwarsa)}.`)
      } catch (err) {
        toast.dariGalat('Cek ulang gagal.', err)
      } finally {
        setSedangCek((s) => {
          const n = new Set(s)
          n.delete(it.id)
          return n
        })
        void segarkan()
      }
    },
    [toast, segarkan],
  )

  const aksi = useMemo<Aksi>(
    () => ({
      tambah: (opsi: BukaTambah = {}) => setSheet({ mode: 'tambah', awal: opsi }),
      ubah: (item, fokus) => setSheet({ mode: 'ubah', item, fokus }),
      hapus: (item) => setHapus(item),
      cekUlang: (item) => void cekUlang(item),
      sedangCek,
      tambahKlien: () => setDialogKlien(null),
      ubahKlien: (k) => setDialogKlien(k),
      hapusKlien: (k) => setHapusKlien(k),
    }),
    [cekUlang, sedangCek],
  )

  // "/" focuses the search, "N" opens Tambah aset (not while typing).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey || adaModal() || sedangMengetik(e.target)) return
      if (e.key === '/') {
        const el = document.getElementById('cari')
        if (el) {
          e.preventDefault()
          el.focus()
        }
      } else if ((e.key === 'n' || e.key === 'N') && rute.nama !== 'pengaturan') {
        e.preventDefault()
        const k = klienRute ? kq.data?.find((x) => x.id === klienRute) : undefined
        setSheet({ mode: 'tambah', awal: k ? { klien: k } : {} })
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [rute.nama, klienRute, kq.data])

  const items = q.data ?? []
  const klien = kq.data ?? []
  const muat: Muat =
    q.data && kq.data
      ? { status: 'siap' }
      : q.isError || kq.isError
        ? {
            status: 'galat',
            pesan: pesanGalat(q.error ?? kq.error),
            cobaLagi: () => void Promise.all([q.refetch(), kq.refetch()]),
          }
        : { status: 'memuat' }

  const props = { items, klien, saring, setSaring, aksi, muat, akun, rute }
  useJudulHalaman(
    rute.nama === 'pengaturan'
      ? 'Pengaturan'
      : rute.nama === 'klien'
        ? (klien.find((x) => x.id === rute.id)?.nama ?? 'Klien')
        : 'Ringkasan',
  )

  return (
    <>
      {phone ? <Phone {...props} /> : <Desktop {...props} />}
      {sheet ? (
        <AssetSheet
          key={sheet.mode === 'ubah' ? sheet.item.id : 'baru'}
          buka={sheet}
          phone={phone}
          klien={klien}
          onClose={() => setSheet(null)}
          onSaved={(it, baru) => {
            setSheet(null)
            toast.ok(baru ? 'Aset ditambahkan.' : 'Perubahan disimpan.', `${it.nama} berakhir ${tanggal(it.kedaluwarsa)}.`)
            void segarkan()
          }}
          onHapus={(it) => setHapus(it)}
        />
      ) : null}
      {hapus ? (
        <ConfirmDialog
          judul="Hapus aset ini?"
          tombol="Hapus"
          tombolSedang="Menghapus"
          sedang={hapusM.isPending}
          onBatal={() => setHapus(null)}
          onYa={() => hapusM.mutate(hapus)}
        >
          <b>{hapus.nama}</b> hilang dari daftar dan dari kalender. Aset yang dihapus tidak bisa dikembalikan.
        </ConfirmDialog>
      ) : null}
      {dialogKlien !== undefined ? (
        <KlienDialog
          key={dialogKlien?.id ?? 'baru'}
          klien={dialogKlien ?? undefined}
          onClose={() => setDialogKlien(undefined)}
          onSaved={(k, baru) => {
            setDialogKlien(undefined)
            toast.ok(baru ? 'Klien ditambahkan.' : 'Perubahan disimpan.', k.nama)
            if (baru) pergi(jalurKlien(k.id))
            void segarkan()
          }}
        />
      ) : null}
      {hapusKlien ? (
        <ConfirmDialog
          judul="Hapus klien ini?"
          tombol="Hapus klien"
          tombolSedang="Menghapus"
          sedang={hapusKlienM.isPending}
          onBatal={() => setHapusKlien(null)}
          onYa={() => hapusKlienM.mutate(hapusKlien)}
        >
          <b>{hapusKlien.nama}</b> akan dihapus.{' '}
          {hapusKlien.jumlah_aset > 0
            ? `${hapusKlien.jumlah_aset} aset dan tugasnya tetap tersimpan, tanpa klien.`
            : 'Tugasnya tetap tersimpan, tanpa klien.'}{' '}
          Aktivitas, ringkasan laporan, dan tautan laporannya tidak bisa dibuka lagi.
        </ConfirmDialog>
      ) : null}
      {hapusAkun ? (
        <ConfirmDialog
          judul="Hapus akun Anda?"
          tombol="Hapus akun"
          tombolSedang="Menghapus"
          sedang={hapusAkunM.isPending}
          onBatal={() => setHapusAkun(false)}
          onYa={() => hapusAkunM.mutate()}
        >
          Akun, klien, aset, tugas, dan catatan pekerjaan Anda akan dihapus dan tidak bisa dikembalikan. Anda masuk sebagai{' '}
          <b>{pengguna.email}</b>.
        </ConfirmDialog>
      ) : null}
    </>
  )
}
