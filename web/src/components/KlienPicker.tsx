// Client picker in the asset sheet: a combobox over the user's clients with
// "Tanpa klien" first and, for a name that is not a client yet, "Tambah klien".
// A new name is sent with the asset and the API creates the client on save.
import { useId, useMemo, useRef, useState } from 'react'
import type { Klien } from '../lib/api'
import { useLetakDaftar } from '../hooks/useLetakDaftar'
import { Icon } from './Icon'
import '../styles/klien.css'

/** id set: an existing client. id null and nama set: a new client. Both empty: no client. */
export interface PilihanKlien {
  id: string | null
  nama: string
}

type Opsi = { kind: 'tanpa' } | { kind: 'klien'; klien: Klien } | { kind: 'baru'; nama: string }

interface Props {
  id: string
  klien: Klien[]
  value: PilihanKlien
  onChange: (v: PilihanKlien) => void
  invalid?: boolean
  describedBy?: string
}

const sama = (a: string, b: string) => a.localeCompare(b, 'id', { sensitivity: 'accent' }) === 0

export function KlienPicker({ id, klien, value, onChange, invalid, describedBy }: Props) {
  const listId = useId()
  const inputRef = useRef<HTMLInputElement>(null)
  const [teks, setTeks] = useState(value.nama)
  const [buka, setBuka] = useState(false)
  const listRef = useRef<HTMLUListElement>(null)
  const [aktif, setAktif] = useState(0)
  // Filter only after the user types; on open every client is listed.
  const [mengetik, setMengetik] = useState(false)

  const opsi = useMemo<Opsi[]>(() => {
    const q = mengetik ? teks.trim().toLowerCase() : ''
    const cocok = klien.filter((k) => !q || k.nama.toLowerCase().includes(q))
    const out: Opsi[] = [{ kind: 'tanpa' }, ...cocok.map((k) => ({ kind: 'klien' as const, klien: k }))]
    const t = teks.trim()
    if (mengetik && t && !klien.some((k) => sama(k.nama, t))) out.push({ kind: 'baru', nama: t.slice(0, 80) })
    return out
  }, [klien, teks, mengetik])

  function bukaDaftar() {
    setBuka(true)
    setMengetik(false)
    const i = value.id ? opsi.findIndex((o) => o.kind === 'klien' && o.klien.id === value.id) : 0
    setAktif(Math.max(0, i))
  }

  function pilih(o: Opsi) {
    const v: PilihanKlien =
      o.kind === 'tanpa' ? { id: null, nama: '' } : o.kind === 'klien' ? { id: o.klien.id, nama: o.klien.nama } : { id: null, nama: o.nama }
    onChange(v)
    setTeks(v.nama)
    setBuka(false)
    setMengetik(false)
  }

  // Leaving the field with typed text keeps what the text means: an existing
  // client by name, no client when empty, otherwise a new client.
  function selesai() {
    setBuka(false)
    if (!mengetik) return
    setMengetik(false)
    const t = teks.trim()
    const ada = klien.find((k) => sama(k.nama, t))
    const v: PilihanKlien = !t ? { id: null, nama: '' } : ada ? { id: ada.id, nama: ada.nama } : { id: null, nama: t.slice(0, 80) }
    onChange(v)
    setTeks(v.nama)
  }

  const sorot = Math.min(aktif, opsi.length - 1)
  useLetakDaftar(buka, listRef, opsi.length)
  const optId = (i: number) => `${listId}-o${i}`
  const terpilih = (o: Opsi) =>
    o.kind === 'tanpa' ? !value.id && !value.nama : o.kind === 'klien' ? o.klien.id === value.id : false

  return (
    <div className="picker">
      <input
        ref={inputRef}
        id={id}
        className="control is-combo"
        role="combobox"
        aria-expanded={buka}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={buka ? optId(sorot) : undefined}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        placeholder="Tanpa klien"
        autoComplete="off"
        spellCheck={false}
        maxLength={80}
        value={teks}
        onFocus={bukaDaftar}
        onClick={() => (buka ? undefined : bukaDaftar())}
        onBlur={selesai}
        onChange={(e) => {
          setTeks(e.target.value)
          setMengetik(true)
          setBuka(true)
          setAktif(e.target.value.trim() ? 1 : 0)
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault()
            if (!buka) return bukaDaftar()
            const d = e.key === 'ArrowDown' ? 1 : -1
            setAktif((sorot + d + opsi.length) % opsi.length)
          } else if (e.key === 'Enter') {
            if (!buka) return
            e.preventDefault()
            pilih(opsi[sorot])
          } else if (e.key === 'Escape') {
            if (!buka) return
            e.preventDefault()
            setBuka(false)
            setMengetik(false)
            setTeks(value.nama)
          } else if (e.key === 'Tab') {
            setBuka(false)
          }
        }}
      />
      {buka ? (
        <ul ref={listRef} className="picker__list" id={listId} role="listbox" aria-label="Klien">
          {opsi.map((o, i) => (
            <li
              key={o.kind === 'klien' ? o.klien.id : o.kind}
              id={optId(i)}
              role="option"
              aria-selected={terpilih(o)}
              className={i === sorot ? 'picker__opt is-active' : 'picker__opt'}
              onMouseDown={(e) => e.preventDefault()}
              onMouseEnter={() => setAktif(i)}
              onClick={() => pilih(o)}
            >
              {o.kind === 'tanpa' ? (
                <span className="picker__muted">Tanpa klien</span>
              ) : o.kind === 'klien' ? (
                <>
                  <span className="picker__name">{o.klien.nama}</span>
                  <span className="picker__meta num">{o.klien.jumlah_aset} aset</span>
                </>
              ) : (
                <>
                  <Icon name="plus" />
                  <span className="picker__name">Tambah klien “{o.nama}”</span>
                </>
              )}
              <Icon name="tick" className={terpilih(o) ? 'picker__check' : 'picker__check is-hidden'} />
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
