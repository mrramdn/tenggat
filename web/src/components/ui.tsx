// Small shared pieces: the status pill and the big day count.
import type { Status } from '../lib/api'
import { kelasStatus, labelStatus, satuanHari } from '../lib/format'

export function Pill({ status }: { status: Status }) {
  return (
    <span className={`pill st-${kelasStatus[status]}`}>
      <span className="dot" />
      {labelStatus[status]}
    </span>
  )
}

export function Days({ n }: { n: number }) {
  return (
    <div className={n < 0 ? 'days is-lewat' : 'days'}>
      <span className="days__n num">{Math.abs(n)}</span>
      <span className="days__u">{satuanHari(n)}</span>
    </div>
  )
}
