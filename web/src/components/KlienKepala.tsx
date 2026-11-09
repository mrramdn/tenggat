// Header of the chosen client above the list: name, note, figures and actions.
import type { Klien } from '../lib/api'
import { rupiahBiasa, tanggal } from '../lib/format'
import { Icon } from './Icon'
import type { Aksi } from './types'
import '../styles/klien.css'

interface Props {
  klien: Klien
  aksi: Aksi
  phone?: boolean
  /** Phone only: clear the client filter. */
  onTutup?: () => void
}

export function KlienKepala({ klien: k, aksi, phone = false, onTutup }: Props) {
  const ukuran = phone ? 'is-lg' : 'is-sm'
  return (
    <section className={phone ? 'kk is-phone' : 'kk'} aria-labelledby="kk-nama">
      <div className="kk__text">
        <div className="kk__top">
          <h2 className="kk__nama" id="kk-nama">
            {k.nama}
          </h2>
          {onTutup ? (
            <button type="button" className={`btn btn--ghost btn--icon ${ukuran}`} aria-label="Tampilkan semua klien" onClick={onTutup}>
              <Icon name="x" />
            </button>
          ) : null}
        </div>
        {k.catatan ? <p className="kk__catatan">{k.catatan}</p> : null}
        <ul className="kk__fakta">
          <li>
            <b className="num">{k.jumlah_aset}</b> aset
          </li>
          <li>
            {k.biaya_30_hari > 0 ? (
              <>
                <b className="num">{rupiahBiasa(k.biaya_30_hari)}</b> jatuh tempo dalam 30 hari
              </>
            ) : (
              'Tidak ada biaya dalam 30 hari'
            )}
          </li>
          {k.terdekat ? (
            <li>
              Tenggat terdekat <b className="num">{tanggal(k.terdekat)}</b>
            </li>
          ) : null}
        </ul>
      </div>
      <div className="kk__aksi">
        <button type="button" className={`btn btn--secondary ${ukuran}`} onClick={() => aksi.tambah({ klien: k })}>
          <Icon name="plus" />
          Tambah aset
        </button>
        <button type="button" className={`btn btn--secondary ${ukuran}`} onClick={() => aksi.ubahKlien(k)}>
          <Icon name="pencil" />
          Ubah
        </button>
        {phone ? (
          <button type="button" className="btn btn--ghost btn--icon is-lg" aria-label="Hapus klien" title="Hapus klien" onClick={() => aksi.hapusKlien(k)}>
            <Icon name="trash" />
          </button>
        ) : (
          <button type="button" className="btn btn--ghost is-sm" onClick={() => aksi.hapusKlien(k)}>
            <Icon name="trash" />
            Hapus
          </button>
        )}
      </div>
    </section>
  )
}
