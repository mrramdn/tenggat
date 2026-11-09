// Pengaturan (/app/pengaturan): calendar subscription, notifications and the account.
import type { ReactNode } from 'react'
import type { Akun } from './Akun'
import { inisial } from './Akun'
import { Icon } from './Icon'
import { KalenderPengaturan } from './KalenderPengaturan'
import { PengaturanNotifikasi } from './PengaturanNotifikasi'

function Bagian({ id, judul, teks, children }: { id: string; judul: string; teks: string; children: ReactNode }) {
  return (
    <section className="set" aria-labelledby={id}>
      <div className="set__kepala">
        <h2 className="set__judul" id={id}>
          {judul}
        </h2>
        <p className="set__teks">{teks}</p>
      </div>
      <div className="set__isi">{children}</div>
    </section>
  )
}

export function Pengaturan({ akun, phone = false }: { akun: Akun; phone?: boolean }) {
  const { pengguna } = akun
  return (
    <div className={phone ? 'hal-set is-phone' : 'hal-set'}>
      <h1 className="hal-set__judul">Pengaturan</h1>
      <Bagian
        id="set-kalender"
        judul="Kalender"
        teks="Aplikasi kalender membaca tanggal berakhir aset Anda dari tautan langganan dan memperbaruinya sendiri."
      >
        <KalenderPengaturan />
      </Bagian>
      <Bagian
        id="set-notifikasi"
        judul="Notifikasi"
        teks="Setiap pukul 08.00 WIB Tenggat mengirim ringkasan ke Slack, Discord, atau Telegram bila ada aset atau tugas yang sudah lewat atau jatuh tempo dalam 7 hari. Pada hari tanpa tenggat dekat, tidak ada pesan. URL webhook disimpan hanya untuk mengirim pesan ini."
      >
        <PengaturanNotifikasi />
      </Bagian>
      <Bagian id="set-akun" judul="Akun" teks="Akun yang sedang masuk di perangkat ini.">
        <div className="set-akun">
          <span className="avatar" aria-hidden="true">
            {inisial(pengguna.nama)}
          </span>
          <span className="akun__teks">
            <span className="akun__nama">{pengguna.nama}</span>
            <span className="akun__email">{pengguna.email}</span>
          </span>
          <button type="button" className="btn btn--secondary" onClick={akun.keluar} disabled={akun.sedangKeluar} aria-busy={akun.sedangKeluar}>
            <Icon name={akun.sedangKeluar ? 'loader' : 'log-out'} className={akun.sedangKeluar ? 'spin' : undefined} />
            {akun.sedangKeluar ? 'Sedang keluar' : 'Keluar'}
          </button>
        </div>
        <div className="set-bahaya">
          <div>
            <b>Hapus akun</b>
            <p>Akun, klien, aset, tugas, dan catatan pekerjaan Anda dihapus dan tidak bisa dikembalikan.</p>
          </div>
          <button type="button" className="btn btn--secondary is-danger" onClick={akun.hapus}>
            <Icon name="trash" />
            Hapus akun
          </button>
        </div>
      </Bagian>
    </div>
  )
}
