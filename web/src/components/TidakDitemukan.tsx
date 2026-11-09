// An address that is not a page of Tenggat.
import { Tautan } from '../lib/rute'
import { Icon } from './Icon'

export function TidakDitemukan({ sudahMasuk }: { sudahMasuk: boolean }) {
  return (
    <div className="boot is-tengah">
      <main className="state">
        <Tautan ke="/" className="wordmark">
          tenggat
        </Tautan>
        <h1 className="display">Halaman tidak ditemukan.</h1>
        <p>Alamat ini tidak ada di Tenggat. Periksa lagi tautannya.</p>
        <Tautan ke={sudahMasuk ? '/app' : '/'} className="btn btn--primary">
          <Icon name="arrow-left" />
          {sudahMasuk ? 'Ke ringkasan' : 'Ke halaman depan'}
        </Tautan>
      </main>
    </div>
  )
}
