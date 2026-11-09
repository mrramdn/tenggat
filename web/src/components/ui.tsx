// Small shared pieces: the status pill and the big day count.
import type { Status, StatusKlien } from '../lib/api'
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

const LABEL_KLIEN: Record<StatusKlien, string> = { aktif: 'Aktif', jeda: 'Jeda', selesai: 'Selesai' }

/** A client's status: aktif, jeda (paused) or selesai (no longer a client). */
export function PillKlien({ status }: { status: StatusKlien }) {
  return (
    <span className={`pill pill--klien is-${status}`}>
      <span className="dot" />
      {LABEL_KLIEN[status]}
    </span>
  )
}

export const labelStatusKlien = LABEL_KLIEN
