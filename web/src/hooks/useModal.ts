import { useEffect, useRef, type RefObject } from 'react'

// Open modals, the last one on top; only the top one handles keys.
const tumpukan: object[] = []

/** True while any sheet or dialog is open. */
export const adaModal = () => tumpukan.length > 0

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

const isianTeks = (n: HTMLElement) =>
  n instanceof HTMLTextAreaElement ||
  (n instanceof HTMLInputElement && !['button', 'checkbox', 'radio', 'submit', 'reset', 'file', 'color', 'range'].includes(n.type))

/**
 * Modal behaviour for a dialog element: moves focus inside on open (to the
 * element marked data-autofocus, else the first focusable one), keeps Tab
 * inside, closes on Escape and gives focus back to the opener on close.
 */
export function useModal(ref: RefObject<HTMLElement | null>, onClose: () => void) {
  const tutup = useRef(onClose)
  tutup.current = onClose

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const token = {}
    tumpukan.push(token)
    const opener = document.activeElement as HTMLElement | null
    const first = el.querySelector<HTMLElement>('[data-autofocus]') ?? el.querySelector<HTMLElement>(FOCUSABLE)
    // On a touch screen a focused text field opens the keyboard over half the dialog. Focus the dialog itself
    // instead, unless the dialog is a form to fill in right away (data-ketik-langsung, e.g. a new asset).
    const sentuh = window.matchMedia?.('(pointer: coarse)').matches ?? false
    if (sentuh && first && isianTeks(first) && !el.hasAttribute('data-ketik-langsung')) {
      if (!el.hasAttribute('tabindex')) el.tabIndex = -1
      el.focus()
    } else {
      ;(first ?? el).focus()
    }

    function onKey(e: KeyboardEvent) {
      if (!el || tumpukan[tumpukan.length - 1] !== token) return
      if (e.key === 'Escape') {
        // An open combobox list closes first; the combobox handles that Escape itself.
        const t = e.target
        if (t instanceof HTMLElement && t.getAttribute('role') === 'combobox' && t.getAttribute('aria-expanded') === 'true') return
        e.preventDefault()
        e.stopPropagation()
        tutup.current()
        return
      }
      if (e.key !== 'Tab') return
      const list = [...el.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((n) => n.offsetParent !== null)
      if (list.length === 0) return
      const a = list[0]
      const z = list[list.length - 1]
      if (e.shiftKey && (document.activeElement === a || !el.contains(document.activeElement))) {
        e.preventDefault()
        z.focus()
      } else if (!e.shiftKey && document.activeElement === z) {
        e.preventDefault()
        a.focus()
      }
    }
    document.addEventListener('keydown', onKey, true)
    return () => {
      document.removeEventListener('keydown', onKey, true)
      tumpukan.splice(tumpukan.indexOf(token), 1)
      if (opener && document.contains(opener)) opener.focus()
    }
  }, [ref])
}
