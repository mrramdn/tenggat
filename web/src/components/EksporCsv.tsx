// "Ekspor CSV": downloads every asset as a spreadsheet file through the API helper, so an expired session goes back
// to the sign-in screen and other errors show as a toast.
import { useState, type ButtonHTMLAttributes } from 'react'
import { api } from '../lib/api'
import { Icon } from './Icon'
import { useToast } from './Toasts'

type Props = Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'onClick' | 'type' | 'children'> & {
  /** Show only the icon (phone header); the label then goes to aria-label. */
  ikonSaja?: boolean
}

export function EksporCsv({ ikonSaja = false, className, ...props }: Props) {
  const toast = useToast()
  const [sedang, setSedang] = useState(false)
  async function unduh() {
    if (sedang) return
    setSedang(true)
    try {
      await api.unduhCsv()
    } catch (err) {
      toast.dariGalat('CSV tidak dapat diunduh.', err)
    } finally {
      setSedang(false)
    }
  }
  return (
    <button
      type="button"
      {...props}
      className={sedang ? `${className ?? ''} is-busy` : className}
      aria-busy={sedang}
      aria-label={ikonSaja ? 'Ekspor CSV' : undefined}
      title={ikonSaja ? 'Ekspor CSV' : undefined}
      onClick={() => void unduh()}
    >
      <Icon name={sedang ? 'loader' : 'download'} className={sedang ? 'spin' : undefined} />
      {ikonSaja ? null : 'Ekspor CSV'}
    </button>
  )
}
