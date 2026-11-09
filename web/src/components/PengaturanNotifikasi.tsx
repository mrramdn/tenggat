// Notifications through the owner's own webhook (Slack, Discord or Telegram): a daily summary at 08.00 WIB when
// something is overdue or due within 7 days. The API never returns the full URL, only a masked destination.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useRef, useState, type FormEvent } from 'react'
import { ApiError } from '../lib/api'
import { KUNCI_NOTIFIKASI, notifikasiApi, type Layanan, type Notifikasi } from '../lib/api-laporan'
import { lalu, tanggal } from '../lib/format'
import { Icon } from './Icon'
import { IkonLaporan } from './IkonLaporan'
import { pesanGalat, useToast } from './Toasts'
import '../styles/laporan.css'

const namaLayanan: Record<Layanan, string> = { slack: 'Slack', discord: 'Discord', telegram: 'Telegram' }

/** The service a pasted URL belongs to, by the same rules as the API's allowlist; null when it is none of them. */
export function kenaliLayanan(teks: string): Layanan | null {
  let u: URL
  try {
    u = new URL(teks.trim())
  } catch {
    return null
  }
  if (u.protocol !== 'https:') return null
  const host = u.hostname.toLowerCase()
  if (host === 'hooks.slack.com' && u.pathname.startsWith('/services/')) return 'slack'
  if ((host === 'discord.com' || host === 'discordapp.com') && u.pathname.startsWith('/api/webhooks/')) return 'discord'
  if (host === 'api.telegram.org' && /^\/bot[^/]+\/sendMessage$/.test(u.pathname) && u.searchParams.get('chat_id'))
    return 'telegram'
  return null
}

/** "10 Nov 2026" for a date, "3 jam lalu" for a timestamp. */
function waktuTerakhir(s: string): string {
  return /^\d{4}-\d{2}-\d{2}$/.test(s) ? tanggal(s) : lalu(s)
}

export function PengaturanNotifikasi() {
  const uid = useId()
  const qc = useQueryClient()
  const toast = useToast()
  const q = useQuery({ queryKey: KUNCI_NOTIFIKASI, queryFn: notifikasiApi.ambil })

  const [ubah, setUbah] = useState(false)
  const [url, setUrl] = useState('')
  const [galat, setGalat] = useState<string | null>(null)
  const [konfirmasi, setKonfirmasi] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const simpan = useMutation({
    mutationFn: (u: string) => notifikasiApi.simpan(u),
    onSuccess: (n) => {
      if (n && typeof n === 'object' && 'aktif' in n) qc.setQueryData<Notifikasi>(KUNCI_NOTIFIKASI, n)
      void qc.invalidateQueries({ queryKey: KUNCI_NOTIFIKASI })
      setUrl('')
      setUbah(false)
      setGalat(null)
      toast.ok('Notifikasi aktif.', 'Kirim pesan uji untuk memastikan pesan sampai.')
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 422) {
        setGalat(err.field.webhook_url ?? err.message)
        inputRef.current?.focus()
      } else toast.dariGalat('Notifikasi tidak dapat disimpan.', err)
    },
  })

  const matikan = useMutation({
    mutationFn: notifikasiApi.matikan,
    onSuccess: () => {
      qc.setQueryData<Notifikasi>(KUNCI_NOTIFIKASI, { aktif: false, layanan: null, tujuan: null, terakhir: null })
      void qc.invalidateQueries({ queryKey: KUNCI_NOTIFIKASI })
      setKonfirmasi(false)
      toast.ok('Notifikasi dimatikan.', 'Tenggat tidak lagi mengirim ringkasan harian.')
    },
    onError: (err) => toast.dariGalat('Notifikasi tidak dapat dimatikan.', err),
  })

  const uji = useMutation({
    mutationFn: notifikasiApi.uji,
    onSuccess: () => toast.ok('Pesan uji terkirim.', 'Periksa kanal atau obrolan tujuan.'),
    onError: (err) => toast.dariGalat('Pesan uji tidak terkirim.', err),
  })

  function kirim(e: FormEvent) {
    e.preventDefault()
    const u = url.trim()
    if (!u) {
      setGalat('Tempel URL webhook terlebih dahulu.')
      inputRef.current?.focus()
      return
    }
    if (!kenaliLayanan(u)) {
      setGalat('URL ini bukan webhook Slack, Discord, atau Telegram. Periksa contoh di bawah.')
      inputRef.current?.focus()
      return
    }
    setGalat(null)
    simpan.mutate(u)
  }

  const n = q.data
  const terdeteksi = kenaliLayanan(url)

  const formulir = (
    <form className="notif__form field" onSubmit={kirim} noValidate>
      <label className="field__label" htmlFor={`${uid}-url`}>
        URL webhook
      </label>
      <div className="field-row">
        <input
          ref={inputRef}
          id={`${uid}-url`}
          className="control notif__url"
          type="url"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          placeholder="https://"
          value={url}
          aria-invalid={galat ? true : undefined}
          aria-describedby={`${uid}-hint${galat ? ` ${uid}-galat` : ''}`}
          onChange={(e) => {
            setUrl(e.target.value)
            if (galat) setGalat(null)
          }}
        />
        <button type="submit" className="btn btn--primary" disabled={simpan.isPending} aria-busy={simpan.isPending}>
          {simpan.isPending ? <Icon name="loader" className="spin" /> : null}
          {simpan.isPending ? 'Menyimpan' : 'Simpan'}
        </button>
      </div>
      {galat ? (
        <div className="field__error" id={`${uid}-galat`} role="alert">
          <Icon name="alert" />
          <span>{galat}</span>
        </div>
      ) : terdeteksi ? (
        <div className="field__hint">
          <span className="notif__terdeteksi">
            <Icon name="tick" />
            Webhook {namaLayanan[terdeteksi]}
          </span>
        </div>
      ) : null}
      <ul className="notif__petunjuk" id={`${uid}-hint`}>
        <li>
          <b>Slack</b>: Incoming Webhook, diawali <code>hooks.slack.com/services/</code>
        </li>
        <li>
          <b>Discord</b>: Pengaturan kanal, Integrasi, Webhook, lalu Salin URL Webhook.
        </li>
        <li>
          <b>Telegram</b>: <code>api.telegram.org/bot&lt;token&gt;/sendMessage?chat_id=&lt;id&gt;</code>
        </li>
      </ul>
      {ubah ? (
        <div>
          <button
            type="button"
            className="btn-teks notif__batal"
            onClick={() => {
              setUbah(false)
              setUrl('')
              setGalat(null)
            }}
          >
            Batal mengganti
          </button>
        </div>
      ) : null}
    </form>
  )

  let isi
  if (q.isPending) {
    isi = <span className="sk" style={{ width: '100%', height: 36 }} aria-label="Memuat" />
  } else if (q.isError) {
    isi = (
      <div className="form-error" role="alert">
        <Icon name="alert" />
        <span>
          Pengaturan notifikasi tidak dapat dimuat. {pesanGalat(q.error)}{' '}
          <button type="button" className="btn-teks is-danger" onClick={() => void q.refetch()}>
            Coba lagi
          </button>
        </span>
      </div>
    )
  } else if (n?.aktif && !ubah) {
    isi = (
      <div className="notif__aktif" aria-live="polite">
        <div className="notif__baris">
          <span className="notif__ikon" aria-hidden="true">
            <IkonLaporan name="bell" />
          </span>
          <div className="notif__layanan">
            <b>Aktif lewat {n.layanan ? namaLayanan[n.layanan] : 'webhook'}</b>
            <span className="notif__tujuan">{n.tujuan}</span>
            <span className="notif__meta">
              {n.terakhir
                ? `Pemeriksaan harian terakhir ${waktuTerakhir(n.terakhir)}.`
                : 'Pemeriksaan harian pertama berjalan pukul 08.00 WIB.'}
            </span>
          </div>
        </div>
        {konfirmasi ? (
          <div className="notif__konfirmasi" role="group" aria-labelledby={`${uid}-k`}>
            <p id={`${uid}-k`}>
              Tenggat berhenti mengirim ringkasan harian dan menghapus URL webhook. Untuk menyalakan lagi, Anda perlu
              menempel URL-nya kembali.
            </p>
            <div className="notif__aksi">
              <button type="button" className="btn btn--secondary" onClick={() => setKonfirmasi(false)} disabled={matikan.isPending} autoFocus>
                Batal
              </button>
              <button type="button" className="btn btn--danger" onClick={() => matikan.mutate()} disabled={matikan.isPending} aria-busy={matikan.isPending}>
                <Icon name={matikan.isPending ? 'loader' : 'trash'} className={matikan.isPending ? 'spin' : undefined} />
                {matikan.isPending ? 'Mematikan' : 'Matikan notifikasi'}
              </button>
            </div>
          </div>
        ) : (
          <div className="notif__aksi">
            <button type="button" className="btn btn--secondary" onClick={() => uji.mutate()} disabled={uji.isPending} aria-busy={uji.isPending}>
              {uji.isPending ? <Icon name="loader" className="spin" /> : <IkonLaporan name="send" />}
              {uji.isPending ? 'Mengirim' : 'Kirim pesan uji'}
            </button>
            <button type="button" className="btn btn--secondary" onClick={() => setUbah(true)}>
              <Icon name="pencil" />
              Ganti URL
            </button>
            <button type="button" className="btn-teks is-danger" onClick={() => setKonfirmasi(true)}>
              Matikan
            </button>
          </div>
        )}
      </div>
    )
  } else {
    isi = formulir
  }

  return (
    <section className="notif" aria-labelledby={`${uid}-j`}>
      <div className="notif__kepala">
        <h2 className="notif__judul" id={`${uid}-j`}>
          Notifikasi
        </h2>
        <p className="notif__teks">
          Setiap pukul 08.00 WIB Tenggat mengirim ringkasan ke Slack, Discord, atau Telegram bila ada aset atau tugas
          yang sudah lewat atau jatuh tempo dalam 7 hari. Pada hari tanpa tenggat dekat, tidak ada pesan. URL webhook
          disimpan hanya untuk mengirim pesan ini.
        </p>
      </div>
      {isi}
    </section>
  )
}
