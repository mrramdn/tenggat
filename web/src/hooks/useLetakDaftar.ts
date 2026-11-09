import { useLayoutEffect, type RefObject } from 'react'

const RUANG_MIN = 160
const TINGGI_MAKS = 248
const JARAK = 8

/** The nearest ancestor that scrolls (a sheet body, a dialog), whose edges clip the list. */
function wadahGulir(el: HTMLElement): HTMLElement | null {
  for (let p = el.parentElement; p && p !== document.body; p = p.parentElement) {
    const o = getComputedStyle(p).overflowY
    if (o === 'auto' || o === 'scroll' || o === 'hidden') return p
  }
  return null
}

/**
 * Places an open dropdown list (.picker__list) inside the visible area: it opens upwards when there is more room
 * above than below, its height is capped to the room it has (so it never runs under a sheet footer or off the
 * screen), it shifts left when it would pass the right edge, and the chosen option is scrolled into view.
 */
export function useLetakDaftar(buka: boolean, listRef: RefObject<HTMLElement | null>, isi: unknown = null) {
  useLayoutEffect(() => {
    const list = listRef.current
    if (!buka || !list) return
    const picker = list.parentElement
    if (!picker) return
    const wadah = wadahGulir(picker)
    const batas = wadah?.getBoundingClientRect()
    const atas = Math.max(0, batas?.top ?? 0)
    const bawah = Math.min(window.innerHeight, batas?.bottom ?? window.innerHeight)
    const r = picker.getBoundingClientRect()
    const ruangBawah = bawah - r.bottom - JARAK
    const ruangAtas = r.top - atas - JARAK
    const tinggi = Math.min(list.scrollHeight, TINGGI_MAKS)

    const keAtas = tinggi > ruangBawah && ruangAtas > ruangBawah
    list.classList.toggle('is-atas', keAtas)
    const ruang = keAtas ? ruangAtas : ruangBawah
    list.style.maxHeight = `${Math.max(Math.min(ruang, TINGGI_MAKS), Math.min(RUANG_MIN, tinggi))}px`

    list.style.left = ''
    const kanan = list.getBoundingClientRect().right
    const tepi = Math.min(window.innerWidth, wadah?.getBoundingClientRect().right ?? window.innerWidth) - JARAK
    if (kanan > tepi) list.style.left = `${Math.round(tepi - kanan)}px`

    const pilihan = list.querySelector<HTMLElement>('[aria-selected="true"]')
    if (pilihan) {
      const t = pilihan.offsetTop
      if (t < list.scrollTop || t + pilihan.offsetHeight > list.scrollTop + list.clientHeight) {
        list.scrollTop = t - (list.clientHeight - pilihan.offsetHeight) / 2
      }
    }
  }, [buka, listRef, isi])
}
