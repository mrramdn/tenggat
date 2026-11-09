// The calendar link. It stays a real link (it can be copied and works without JavaScript), but a click downloads
// through the API helper, so an expired session shows the sign-in screen instead of a file holding the error.
import type { AnchorHTMLAttributes } from 'react'
import { api } from '../lib/api'
import { useToast } from './Toasts'

type Props = Omit<AnchorHTMLAttributes<HTMLAnchorElement>, 'href' | 'download' | 'onClick'>

export function TautanKalender(props: Props) {
  const toast = useToast()
  return (
    <a
      {...props}
      href={api.kalenderUrl}
      download="tenggat.ics"
      onClick={(e) => {
        e.preventDefault()
        api.unduhKalender().catch((err: unknown) => toast.dariGalat('Kalender tidak dapat diunduh.', err))
      }}
    />
  )
}
