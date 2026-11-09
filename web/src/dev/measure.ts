// Dev only (?measure=1): the control-height audit from the mockup's NOTES.md.
// Prints every button, input, select, segmented item and chip grouped by size
// token, the row action column, the overflow buttons and clipped text.
export function ukur() {
  const root = getComputedStyle(document.documentElement)
  const tok: Record<string, number> = {
    sm: parseFloat(root.getPropertyValue('--control-sm')),
    md: parseFloat(root.getPropertyValue('--control-md')),
    lg: parseFloat(root.getPropertyValue('--control-lg')),
  }
  const phone = matchMedia('(max-width: 1023px)').matches
  const out: string[] = [`viewport ${innerWidth}x${innerHeight}${phone ? ' (phone: every control is lg)' : ''}`]
  const groups: Record<string, Record<string, number>> = {}
  let bad = 0
  document.querySelectorAll<HTMLElement>('.btn, .control, .seg__item, .chip').forEach((el) => {
    const r = el.getBoundingClientRect()
    if (r.width === 0 && r.height === 0) return
    const size = phone
      ? 'lg'
      : el.classList.contains('is-sm') || el.closest('.seg')?.classList.contains('is-sm')
        ? 'sm'
        : el.classList.contains('is-lg') || el.classList.contains('chip')
          ? 'lg'
          : 'md'
    const kind = el.classList.contains('btn') ? 'button' : el.classList.contains('chip') ? 'chip' : el.classList.contains('seg__item') ? 'segment' : el.tagName.toLowerCase()
    const cs = getComputedStyle(el)
    let sig = `${Math.round(r.height)}px`
    if (kind === 'button') sig += ` pad ${cs.paddingLeft} r ${cs.borderTopLeftRadius} fs ${cs.fontSize} fw ${cs.fontWeight}`
    if (el.classList.contains('btn--fixed') || el.classList.contains('btn--icon')) sig += ` w ${Math.round(r.width)}`
    const key = `${kind} ${size}`
    groups[key] ??= {}
    groups[key][sig] = (groups[key][sig] ?? 0) + 1
    if (Math.round(r.height) !== tok[size]) {
      bad++
      out.push(`  MISMATCH ${key} "${(el.textContent || (el as HTMLInputElement).placeholder || '').trim()}" ${r.height}`)
    }
  })
  for (const k of Object.keys(groups).sort()) {
    out.push(`  ${k} (token ${tok[k.split(' ')[1]]}px): ${Object.entries(groups[k]).map(([s, n]) => `${s} x${n}`).join(' | ')}`)
  }
  const tally = (sel: string, f: (r: DOMRect) => string) => {
    const m: Record<string, number> = {}
    document.querySelectorAll(sel).forEach((b) => {
      const k = f(b.getBoundingClientRect())
      m[k] = (m[k] ?? 0) + 1
    })
    return JSON.stringify(m)
  }
  out.push(`row action buttons (left,width x height): ${tally('.row .btn--fixed', (r) => `${Math.round(r.left)},${Math.round(r.width)}x${Math.round(r.height)}`)}`)
  out.push(`overflow buttons (left,size): ${tally('.row .btn--icon', (r) => `${Math.round(r.left)},${Math.round(r.width)}x${Math.round(r.height)}`)}`)
  out.push(`mismatches: ${bad}`)
  const tr: string[] = []
  document.querySelectorAll<HTMLElement>('#root *').forEach((el) => {
    if (!el.childNodes.length || !el.textContent?.trim()) return
    const ov = getComputedStyle(el).overflow
    if (el.scrollWidth > el.clientWidth + 1 && (ov === 'hidden' || ov.includes('auto'))) {
      tr.push(`${el.className} ${el.scrollWidth}/${el.clientWidth} "${el.textContent.trim().slice(0, 60)}"`)
    }
  })
  out.push(`truncated or scrolling: ${tr.length ? `\n  ${tr.join('\n  ')}` : 'none'}`)
  const pre = document.createElement('pre')
  pre.id = 'measure'
  pre.textContent = out.join('\n')
  document.body.append(pre)
}
