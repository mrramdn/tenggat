// Stub (shell branch): the beranda agent replaces this with the public front page.
import { Tautan } from '../lib/rute'

export function Beranda({ sudahMasuk }: { sudahMasuk: boolean }) {
  return (
    <main className="stub-hal">
      <div className="wordmark">tenggat</div>
      <p>Tenggat mencatat tanggal berakhir domain, sertifikat, dan langganan situs klien Anda.</p>
      <Tautan ke={sudahMasuk ? '/app' : '/masuk'} className="btn btn--primary">
        {sudahMasuk ? 'Buka aplikasi' : 'Masuk'}
      </Tautan>
    </main>
  )
}
