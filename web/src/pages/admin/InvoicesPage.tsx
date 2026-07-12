import { useEffect, useState } from 'react'
import { useAuth } from '../../auth/AuthProvider'
import { useT, useLang } from '../../i18n/LanguageProvider'
import { useFormatMoney } from '../../money'
import { formatDate } from '../application/helpers'
import { adminGetInvoices, adminPayInvoice, adminVoidInvoice, ApiError } from '../../api/client'
import type { Invoice } from '../../api/types'
import { AdminShell } from './AdminShell'
import '../../styles/billing.css'

const STATUSES = ['', 'pending', 'paid', 'void'] as const

export function InvoicesPage() {
  const { token } = useAuth()
  const t = useT(); const { lang } = useLang()
  const money = useFormatMoney()
  const [invoices, setInvoices] = useState<Invoice[]>([])
  const [status, setStatus] = useState<string>('')
  const [error, setError] = useState('')

  async function reload(s: string) {
    if (!token) return
    try { setInvoices(await adminGetInvoices(token, s || undefined)); setError('') }
    catch (e) { setError(e instanceof ApiError ? e.message : String(e)) }
  }
  useEffect(() => { reload(status) /* eslint-disable-next-line */ }, [token, status])

  async function act(fn: (tk: string, id: number) => Promise<void>, id: number) {
    if (!token) return
    try { await fn(token, id); await reload(status) }
    catch (e) { setError(e instanceof ApiError ? e.message : String(e)) }
  }

  const label = (st: string) => t(`billing.status.${st}`)

  return (
    <AdminShell active="invoices" title={t('billing.admin.title')} description={t('billing.admin.desc')}>
      {error && <p className="err" role="alert">{error}</p>}
      <div className="billing">
        <div className="filters">
          {STATUSES.map(s => (
            <button key={s || 'all'} className={status === s ? 'chip active' : 'chip'} onClick={() => setStatus(s)}>
              {s ? label(s) : t('billing.filterAll')}
            </button>
          ))}
        </div>
        {invoices.length === 0 ? <p className="empty">{t('billing.admin.none')}</p> : (
          <div className="invoices-wrap">
            <table className="invoices">
              <thead><tr>
                <th>{t('billing.col.plan')}</th><th>{t('billing.col.amount')}</th>
                <th>{t('billing.col.team')}</th><th>{t('billing.col.status')}</th>
                <th>{t('billing.col.created')}</th><th></th>
              </tr></thead>
              <tbody>
                {invoices.map(inv => (
                  <tr key={inv.id}>
                    <td>{inv.planName}</td>
                    <td className="amt">{money(inv.priceCents, inv.currency)}</td>
                    <td className="tid">{inv.teamId}</td>
                    <td><span className={`pill ${inv.status}`}>{label(inv.status)}</span></td>
                    <td>{formatDate(inv.createdAt, lang)}</td>
                    <td className="acts">
                      {inv.status === 'pending' && (
                        <>
                          <button className="btn btn-primary btn-sm" onClick={() => act(adminPayInvoice, inv.id)}>{t('billing.admin.pay')}</button>
                          <button className="btn btn-ghost btn-sm" onClick={() => act(adminVoidInvoice, inv.id)}>{t('billing.admin.void')}</button>
                        </>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </AdminShell>
  )
}
