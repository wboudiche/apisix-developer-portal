import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { TopBar } from '../../components/TopBar'
import { useAuth } from '../../auth/AuthProvider'
import { useT, useLang } from '../../i18n/LanguageProvider'
import { useFormatMoney } from '../../money'
import { formatDate } from '../application/helpers'
import { getBillingInvoices, ApiError } from '../../api/client'
import type { Invoice } from '../../api/types'
import '../../styles/billing.css'

export default function BillingPage() {
  const { token } = useAuth()
  const t = useT(); const { lang } = useLang()
  const money = useFormatMoney()
  const [invoices, setInvoices] = useState<Invoice[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    if (!token) return
    getBillingInvoices(token).then(setInvoices).catch(e => setError(e instanceof ApiError ? e.message : String(e)))
  }, [token])

  if (!token) return <Navigate to="/login" replace />

  return (
    <>
    <TopBar search="" onSearch={() => {}} />
    <div className="billing-page">
      <h1>{t('billing.title')}</h1>
      <p className="hint">{t('billing.hint')}</p>
      {error && <p className="err" role="alert">{error}</p>}
      <div className="billing">
        {invoices.length === 0 ? <p className="empty">{t('billing.none')}</p> : (
          <div className="invoices-wrap">
            <table className="invoices">
              <thead><tr>
                <th>{t('billing.col.plan')}</th><th>{t('billing.col.amount')}</th>
                <th>{t('billing.col.status')}</th><th>{t('billing.col.created')}</th>
              </tr></thead>
              <tbody>
                {invoices.map(inv => (
                  <tr key={inv.id}>
                    <td>{inv.planName}</td>
                    <td className="amt">{money(inv.priceCents, inv.currency)}</td>
                    <td><span className={`pill ${inv.status}`}>{t(`billing.status.${inv.status}`)}</span></td>
                    <td>{formatDate(inv.createdAt, lang)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
    </>
  )
}
