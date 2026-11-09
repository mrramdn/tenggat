// 90-day timeline: one lane per asset type, status bands, a "Hari ini" marker,
// month ticks, overdue items at the left edge and later items as an arrow.
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { Item, Jenis } from '../lib/api'
import { frasaHari, kelasStatus, labelLajur, labelStatus, statusDariSisa, tanggalPendek, urutanJenis } from '../lib/format'
import { labelTitik } from '../lib/derive'

const X0 = 76
const PAD_R = 8
const LO = -7
const HI = 90
const TOP = 20
const LANE = 20
const BANDS = [
  { st: 'lewat', a: LO, b: 0 },
  { st: 'kritis', a: 0, b: 8 },
  { st: 'segera', a: 8, b: 31 },
  { st: 'aman', a: 31, b: HI },
] as const

let ctx: CanvasRenderingContext2D | null = null
function lebarTeks(s: string): number {
  if (!ctx) ctx = document.createElement('canvas').getContext('2d')
  if (!ctx) return s.length * 6.6
  ctx.font = '400 12px Geist, ui-sans-serif, system-ui, sans-serif'
  return ctx.measureText(s).width
}

interface Label {
  x: number
  y: number
  anchor: 'start' | 'end'
  teks: string
}

const BULAN = new Intl.DateTimeFormat('id-ID', { day: 'numeric', month: 'short' })

export function Timeline({ items, ghost = false }: { items: Item[]; ghost?: boolean }) {
  const box = useRef<HTMLDivElement>(null)
  const [W, setW] = useState(0)
  const [, setFonts] = useState(0)

  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    setW(Math.round(el.getBoundingClientRect().width))
    const ro = new ResizeObserver(([e]) => setW(Math.round(e.contentRect.width)))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  // Re-measure labels once the web font is in.
  useEffect(() => {
    document.fonts?.ready.then(() => setFonts((n) => n + 1))
  }, [])

  const lajur: Jenis[] = ghost
    ? ['domain', 'sertifikat', 'token', 'langganan']
    : urutanJenis.filter((j) => items.some((it) => it.jenis === j))
  const H = TOP + LANE * lajur.length + 22
  const bottom = TOP + LANE * lajur.length

  if (W === 0) return <div ref={box} className="tl__box" style={{ height: H }} />

  const plot = W - X0 - PAD_R
  const X = (d: number) => X0 + ((d - LO) * plot) / (HI - LO)
  const yLajur = (k: number) => TOP + LANE * k + LANE / 2

  // month ticks: the first day of each month inside the window
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const ticks: { d: number; label: string }[] = []
  for (let k = 1; k <= 4; k++) {
    const t = new Date(today.getFullYear(), today.getMonth() + k, 1)
    const d = Math.round((t.getTime() - today.getTime()) / 86_400_000)
    if (d > 0 && d <= HI) ticks.push({ d, label: BULAN.format(t) })
  }

  const titik: { x: number; y: number; it: Item }[] = []
  const panah: { y: number; it: Item; lagi: number }[] = []
  const labels: Label[] = []

  if (!ghost) {
    lajur.forEach((j, k) => {
      const y = yLajur(k)
      const isi = items.filter((it) => it.jenis === j).sort((a, b) => a.sisa_hari - b.sisa_hari)
      const dalam = isi.filter((it) => it.sisa_hari <= HI)
      const luar = isi.filter((it) => it.sisa_hari > HI)
      const terpakai: [number, number][] = []

      if (luar.length) {
        const it = luar[0]
        panah.push({ y, it, lagi: luar.length - 1 })
        const teks = `${labelTitik(it)} · ${tanggalPendek(it.kedaluwarsa)}${luar.length > 1 ? ` +${luar.length - 1}` : ''}`
        const xe = W - PAD_R - 14
        labels.push({ x: xe, y: y + 4, anchor: 'end', teks })
        terpakai.push([xe - lebarTeks(teks), W])
      }

      const xs = dalam.map((it) => {
        const x = it.sisa_hari < LO ? X(LO) + 5 : X(it.sisa_hari)
        titik.push({ x, y, it })
        return x
      })

      dalam.forEach((it, i) => {
        const x = xs[i]
        let teks = labelTitik(it)
        if (it.sisa_hari < LO) teks = `${teks} · ${tanggalPendek(it.kedaluwarsa)}`
        const w = lebarTeks(teks)
        const kanan: Label & { r: [number, number] } = { x: x + 10, y: y + 4, anchor: 'start', teks, r: [x + 10, x + 10 + w] }
        const kiri: Label & { r: [number, number] } = { x: x - 10, y: y + 4, anchor: 'end', teks, r: [x - 10 - w, x - 10] }
        const urutan = x > X0 + plot * 0.6 ? [kiri, kanan] : [kanan, kiri]
        const lain = xs.filter((_, n) => n !== i).map((p): [number, number] => [p - 6, p + 6])
        const muat = (r: [number, number]) =>
          r[0] >= X0 - 2 &&
          r[1] <= W - PAD_R &&
          [...terpakai, ...lain].every(([a, b]) => r[1] + 4 <= a || r[0] - 4 >= b)
        const pilih = urutan.find((c) => muat(c.r))
        if (pilih) {
          labels.push({ x: pilih.x, y: pilih.y, anchor: pilih.anchor, teks: pilih.teks })
          terpakai.push(pilih.r)
        }
      })
    })
  }

  const ringkas = ghost
    ? 'Linimasa 90 hari, terisi setelah aset pertama'
    : `Linimasa 90 hari: ${items.filter((it) => it.sisa_hari <= HI).length} aset berakhir dalam 90 hari ke depan`

  return (
    <div ref={box} className="tl__box">
      <svg className="tl__svg" viewBox={`0 0 ${W} ${H}`} height={H} role="img" aria-label={ringkas}>
        {BANDS.map(({ st, a, b }) => {
          const cls = kelasStatus[st]
          const tx = st === 'lewat' ? X(b) - 4 : st === 'kritis' ? X(a) + 4 : (X(a) + X(b)) / 2
          const anchor = st === 'lewat' ? 'end' : st === 'kritis' ? 'start' : 'middle'
          return (
            <g key={st}>
              <rect x={X(a)} y={TOP - 2} width={X(b) - X(a)} height={bottom - TOP + 4} fill={`var(--st-${cls}-band)`} />
              <text x={tx} y={12} textAnchor={anchor} style={{ fill: `var(--st-${cls})` }}>
                {labelStatus[st]}
              </text>
            </g>
          )
        })}
        {lajur.map((j, k) => (
          <g key={j}>
            <line x1={X0} x2={W - PAD_R} y1={yLajur(k)} y2={yLajur(k)} stroke="var(--line)" strokeWidth={1} />
            <text className="lane-label" x={0} y={yLajur(k) + 4}>
              {labelLajur[j]}
            </text>
          </g>
        ))}
        {ticks.map((t) => (
          <g key={t.d}>
            <line x1={X(t.d)} x2={X(t.d)} y1={TOP - 2} y2={bottom + 2} stroke="var(--line-strong)" strokeDasharray="2 3" />
            {Math.abs(X(t.d) - X(0)) > 52 && X(t.d) + lebarTeks(t.label) / 2 <= W ? (
              <text x={X(t.d)} y={H - 4} textAnchor="middle">
                {t.label}
              </text>
            ) : null}
          </g>
        ))}
        <line x1={X(0)} x2={X(0)} y1={TOP - 4} y2={bottom + 4} stroke="var(--accent)" strokeWidth={1.5} />
        <text className="today-label" x={X(0)} y={H - 4} textAnchor="middle">
          Hari ini
        </text>
        {panah.map(({ y, it, lagi }) => {
          const xe = W - PAD_R
          return (
            <path
              key={it.id}
              d={`M${xe - 8} ${y - 4} L${xe} ${y} L${xe - 8} ${y + 4}`}
              fill="none"
              stroke={`var(--st-${kelasStatus[statusDariSisa(it.sisa_hari)]})`}
              strokeWidth={1.75}
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <title>{`${it.nama} · ${frasaHari(it.sisa_hari)}${lagi ? ` dan ${lagi} aset lain` : ''}`}</title>
            </path>
          )
        })}
        {titik.map(({ x, y, it }) => (
          <circle key={it.id} cx={x} cy={y} r={4.5} fill={`var(--st-${kelasStatus[it.status]})`} stroke="var(--surface)" strokeWidth={2}>
            <title>{`${it.nama} · ${frasaHari(it.sisa_hari)}`}</title>
          </circle>
        ))}
        {labels.map((l, i) => (
          <text key={i} className="pt-label" x={l.x} y={l.y} textAnchor={l.anchor}>
            {l.teks}
          </text>
        ))}
      </svg>
    </div>
  )
}
