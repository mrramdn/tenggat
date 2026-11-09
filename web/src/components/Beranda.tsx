// The public front page at /: a live lookup of any site's two dates, then the app itself shown in pieces, and
// who made it. Signed-in visitors see "Buka aplikasi" instead of the sign-in links.
import '../styles/beranda.css'
import { CuplikanKalender, CuplikanLaporan, CuplikanRuang } from './beranda/Cuplikan'
import { DemoCek } from './beranda/DemoCek'
import { Icon } from './Icon'
import { Tautan, useJudulHalaman } from '../lib/rute'

const GITHUB = 'https://github.com/mrramdn/tenggat'
const ARTIKEL = 'https://www.mrramdn.com/en/blog/tenggat'

export function Beranda({ sudahMasuk }: { sudahMasuk: boolean }) {
  useJudulHalaman(null)
  return (
    <div className="beranda">
      <a className="beranda__lewati btn btn--secondary is-sm" href="#isi">
        Langsung ke isi
      </a>
      <header className="b-kepala">
        <div className="b-wadah b-kepala__isi">
          <Tautan className="wordmark b-kepala__merek" aria-label="Tenggat, halaman depan" ke="/">
            tenggat
          </Tautan>
          <nav className="b-kepala__aksi" aria-label="Akun">
            {sudahMasuk ? (
              <Tautan className="btn btn--primary is-sm" ke="/app">
                Buka aplikasi
              </Tautan>
            ) : (
              <>
                <Tautan className="btn btn--ghost is-sm b-kepala__masuk" ke="/masuk">
                  Masuk
                </Tautan>
                <Tautan className="btn btn--primary is-sm" ke="/daftar">
                  Buat akun
                </Tautan>
              </>
            )}
          </nav>
        </div>
      </header>

      <main id="isi" tabIndex={-1}>
        <section className="b-wadah b-hero" aria-labelledby="b-judul">
          <div className="b-hero__teks">
            <h1 className="b-hero__judul" id="b-judul">
              Kapan domain dan sertifikat situs klien Anda berakhir?
            </h1>
            <p className="b-hero__lede">
              Ketik alamat situs. Tenggat menanyakan tanggal berakhir domainnya ke registri dan membaca tanggal berakhir
              sertifikatnya langsung dari server. Anda tidak perlu masuk.
            </p>
          </div>
          <DemoCek sudahMasuk={sudahMasuk} />
        </section>

        <section className="b-wadah b-untuk" aria-label="Untuk siapa">
          <p className="b-untuk__teks">
            Tenggat adalah alat kerja untuk freelancer dan agensi kecil di Indonesia yang mengurus situs klien. Setiap
            klien punya halamannya sendiri berisi aset, tugas rutin, dan catatan pekerjaan.{' '}
            <span className="b-untuk__lanjut">
              Di akhir bulan, catatan itu menjadi laporan yang bisa Anda kirim ke klien.
            </span>
          </p>
        </section>

        <section className="b-wadah b-bagian" aria-labelledby="b-ruang">
          <div className="b-bagian__teks">
            <h2 className="b-bagian__judul" id="b-ruang">
              Satu halaman untuk setiap klien
            </h2>
            <p>
              Aset klien ada di halamannya bersama tugas rutin seperti update plugin dan backup. Tandai tugas selesai, dan
              jadwalnya maju ke periode berikutnya.
            </p>
            <p>Pekerjaan yang Anda catat, berikut durasinya, masuk ke log aktivitas klien itu.</p>
          </div>
          <div className="b-bagian__layar">
            <CuplikanRuang />
          </div>
        </section>

        <section className="b-wadah b-bagian" aria-labelledby="b-laporan">
          <div className="b-bagian__teks">
            <h2 className="b-bagian__judul" id="b-laporan">
              Laporan bulanan untuk klien
            </h2>
            <p>
              Laporan disusun dari log pekerjaan dan tanggal aset bulan itu. Tambahkan catatan Anda sendiri, lalu kirim
              tautannya lewat WhatsApp atau simpan sebagai PDF dari browser.
            </p>
            <p>Tautan laporan hanya bisa dibaca, dan bisa Anda cabut kapan saja.</p>
          </div>
          <div className="b-bagian__layar">
            <CuplikanLaporan />
          </div>
        </section>

        <section className="b-wadah b-bagian" aria-labelledby="b-kalender">
          <div className="b-bagian__teks">
            <h2 className="b-bagian__judul" id="b-kalender">
              Tanggalnya ikut ke kalender Anda
            </h2>
            <p>
              Tempel tautan langganan di Google Calendar, Apple Calendar, atau Outlook. Kalender mengambil perubahan
              sendiri setiap kali Anda menambah atau memperpanjang aset.
            </p>
          </div>
          <div className="b-bagian__layar">
            <CuplikanKalender />
          </div>
        </section>

        <section className="b-wadah b-lain" aria-labelledby="b-lain">
          <h2 className="b-bagian__judul" id="b-lain">
            Yang perlu Anda tahu
          </h2>
          <dl className="b-lain__daftar">
            <div>
              <dt>Ringkasan pagi</dt>
              <dd>
                Pukul 08.00 WIB, aset dan tugas yang lewat atau jatuh tempo dalam 7 hari dikirim ke Slack, Discord, atau
                Telegram lewat webhook milik Anda. Bila tidak ada yang perlu dikerjakan, tidak ada kiriman.
              </dd>
            </div>
            <div>
              <dt>Pengingat ke klien</dt>
              <dd>
                Untuk aset yang mendekati tanggalnya, Tenggat menyiapkan pesan WhatsApp yang sopan. Anda memilih kontaknya
                di WhatsApp, Tenggat tidak menyimpan nomor telepon.
              </dd>
            </div>
            <div>
              <dt>Yang disimpan</dt>
              <dd>Nama aset, tanggal berakhir, biaya, dan catatan. Nilai token atau kunci API tidak pernah disimpan.</dd>
            </div>
            <div>
              <dt>Kode sumber</dt>
              <dd>
                Terbuka di{' '}
                <a className="btn-teks" href={GITHUB}>
                  GitHub
                </a>
                . Cara membangun dan menjalankannya di server sendiri ditulis di{' '}
                <a className="btn-teks" href={ARTIKEL}>
                  artikel ini
                </a>
                .
              </dd>
            </div>
          </dl>
        </section>

        <section className="b-wadah b-akhir" aria-label="Mulai">
          {sudahMasuk ? (
            <>
              <p className="b-akhir__teks">Anda sudah masuk.</p>
              <Tautan className="btn btn--primary is-lg" ke="/app">
                Buka aplikasi
              </Tautan>
            </>
          ) : (
            <>
              <p className="b-akhir__teks">Akun Tenggat gratis. Anda hanya perlu nama, email, dan kata sandi.</p>
              <div className="b-akhir__aksi">
                <Tautan className="btn btn--primary is-lg" ke="/daftar">
                  Buat akun
                </Tautan>
                <Tautan className="btn btn--secondary is-lg" ke="/masuk">
                  Masuk
                </Tautan>
              </div>
            </>
          )}
        </section>
      </main>

      <footer className="b-kaki">
        <div className="b-wadah b-kaki__isi">
          <span className="wordmark b-kaki__merek">tenggat</span>
          <span className="b-kaki__oleh">Dibuat oleh Rifki Ramdani</span>
          <nav className="b-kaki__tautan" aria-label="Tautan luar">
            <a href={GITHUB}>
              GitHub
              <Icon name="external-link" />
            </a>
            <a href={ARTIKEL}>
              Artikel
              <Icon name="external-link" />
            </a>
          </nav>
        </div>
      </footer>
    </div>
  )
}
