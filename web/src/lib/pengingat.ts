// A WhatsApp reminder for an asset's client: a wa.me link without a phone number, so the owner picks the chat in
// WhatsApp itself and Tenggat never stores a number.
import type { Item, Jenis } from './api'
import { tanggalPanjang, uang } from './format'

const sebutanJenis: Record<Jenis, string> = {
  sertifikat: 'sertifikat TLS',
  domain: 'domain',
  token: 'token API',
  langganan: 'langganan',
  tagihan: 'tagihan',
  lainnya: 'masa berlaku',
}

/** "domain contoh.co.id", "sertifikat TLS www.contoh.co.id", "masa berlaku Lisensi font". A name that already starts with its
 *  type word ("Domain contoh.co.id", "Token GitHub deploy") keeps its own words, with the type in lower case, so the
 *  type is never said twice. */
export function sebutAset(it: Pick<Item, 'nama' | 'jenis'>): string {
  const jenis = sebutanJenis[it.jenis]
  if (!jenis) return it.nama
  const kata = jenis.split(' ')[0]
  if (it.nama.toLowerCase().startsWith(kata + ' ')) return kata + it.nama.slice(kata.length)
  return `${jenis} ${it.nama}`
}

/** The reminder text, in polite Indonesian. */
export function teksPengingat(it: Item): string {
  const aset = sebutAset(it)
  const untuk = it.klien ? ` untuk ${it.klien}` : ''
  const tgl = tanggalPanjang(it.kedaluwarsa)
  const n = it.sisa_hari
  const tagihan = it.jenis === 'tagihan'

  let kalimat: string
  let minta: string
  if (tagihan) {
    if (n < 0) {
      kalimat = `Saya ingin memberi tahu bahwa ${aset}${untuk} sudah lewat jatuh tempo pada ${tgl}, ${-n} hari yang lalu.`
      minta = 'Mohon konfirmasinya agar dapat segera dibayar.'
    } else if (n === 0) {
      kalimat = `Saya ingin mengingatkan bahwa ${aset}${untuk} jatuh tempo hari ini, ${tgl}.`
      minta = 'Mohon konfirmasinya agar dapat dibayar hari ini.'
    } else {
      const sisa = n === 1 ? 'besok' : `${n} hari lagi`
      kalimat = `Saya ingin mengingatkan bahwa ${aset}${untuk} jatuh tempo pada ${tgl}, ${sisa}.`
      minta = 'Mohon konfirmasinya agar dapat dibayar sebelum tanggal tersebut.'
    }
  } else if (n < 0) {
    kalimat = `Saya ingin memberi tahu bahwa ${aset}${untuk} sudah berakhir pada ${tgl}, ${-n} hari yang lalu.`
    minta = 'Mohon konfirmasinya agar dapat segera diperpanjang.'
  } else if (n === 0) {
    kalimat = `Saya ingin mengingatkan bahwa ${aset}${untuk} berakhir hari ini, ${tgl}.`
    minta = 'Mohon konfirmasinya agar dapat diperpanjang hari ini.'
  } else {
    const sisa = n === 1 ? 'besok' : `${n} hari lagi`
    kalimat = `Saya ingin mengingatkan bahwa ${aset}${untuk} akan berakhir pada ${tgl}, ${sisa}.`
    minta = 'Mohon konfirmasinya agar dapat diperpanjang sebelum tanggal tersebut.'
  }
  const biaya =
    it.biaya != null && it.biaya > 0
      ? tagihan
        ? ` Jumlahnya ${uang(it.biaya, it.mata_uang)}.`
        : ` Biaya perpanjangannya ${uang(it.biaya, it.mata_uang)}.`
      : ''

  return `Halo,\n\n${kalimat}${biaya} ${minta}\n\nTerima kasih.`
}

/** wa.me link that opens WhatsApp with the reminder filled in; the owner chooses the chat. */
export function tautanPengingat(it: Item): string {
  return `https://wa.me/?text=${encodeURIComponent(teksPengingat(it))}`
}
