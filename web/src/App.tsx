import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { AssetSheet, type ModeSheet } from './components/AssetSheet'
import { ConfirmDialog } from './components/ConfirmDialog'
import { Desktop, type Muat } from './components/Desktop'
import { Phone } from './components/Phone'
import { pesanGalat, useToast } from './components/Toasts'
import type { Aksi, BukaTambah } from './components/types'
import { adaModal } from './hooks/useModal'
import { useMediaQuery } from './hooks/useMediaQuery'
import { api, type Item } from './lib/api'
import { daftarKlien, saringAwal, type Saring } from './lib/derive'
import { tanggal } from './lib/format'

const KUNCI = ['item'] as const

function sedangMengetik(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false
  return el.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName)
}

export function App() {
  const phone = useMediaQuery('(max-width: 1023px)')
  const qc = useQueryClient()
  const toast = useToast()
  const q = useQuery({ queryKey: KUNCI, queryFn: () => api.daftar(), refetchInterval: 60_000 })

  const [saring, setSaring] = useState<Saring>(saringAwal)
  const [sheet, setSheet] = useState<ModeSheet | null>(null)
  const [hapus, setHapus] = useState<Item | null>(null)
  const [sedangCek, setSedangCek] = useState<ReadonlySet<string>>(new Set())

  const segarkan = useCallback(() => qc.invalidateQueries({ queryKey: KUNCI }), [qc])

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

  const cekUlang = useCallback(
    async (it: Item) => {
      setSedangCek((s) => new Set(s).add(it.id))
      try {
        const r = await api.cekUlang(it.id)
        if (r.galat_cek) toast.galat('Cek gagal.', `${r.nama}: ${r.galat_cek}`)
        else toast.ok('Sertifikat dicek ulang.', `${r.nama} berakhir ${tanggal(r.kedaluwarsa)}.`)
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
      } else if (e.key === 'n' || e.key === 'N') {
        e.preventDefault()
        setSheet({ mode: 'tambah', awal: {} })
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [])

  const items = q.data ?? []
  const muat: Muat = q.data
    ? { status: 'siap' }
    : q.isError
      ? { status: 'galat', pesan: pesanGalat(q.error), cobaLagi: () => void q.refetch() }
      : { status: 'memuat' }

  const props = { items, saring, setSaring, aksi, muat }

  return (
    <>
      {phone ? <Phone {...props} /> : <Desktop {...props} />}
      {sheet ? (
        <AssetSheet
          key={sheet.mode === 'ubah' ? sheet.item.id : 'baru'}
          buka={sheet}
          phone={phone}
          klien={daftarKlien(items).map((k) => k.nama)}
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
          item={hapus}
          sedang={hapusM.isPending}
          onBatal={() => setHapus(null)}
          onHapus={() => hapusM.mutate(hapus)}
        />
      ) : null}
    </>
  )
}
