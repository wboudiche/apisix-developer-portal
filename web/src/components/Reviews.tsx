import { useEffect, useState } from 'react'
import { getRatings, submitRating } from '../api/client'
import type { RatingsView } from '../api/types'
import { formatRelative } from '../pages/application/activity'
import { useT } from '../i18n/LanguageProvider'

function StarRow({ value }: { value: number }) {
  return <span className="rv-stars" aria-label={`${value}/5`}>{'★★★★★'.slice(0, value)}{'☆☆☆☆☆'.slice(0, 5 - value)}</span>
}

export function Reviews({ slug, token }: { slug: string; token: string | null }) {
  const t = useT()
  const [view, setView] = useState<RatingsView | null>(null)
  const [stars, setStars] = useState(0)
  const [comment, setComment] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  function load() {
    getRatings(slug, token ?? undefined).then(v => {
      setView(v)
      if (v.mine) { setStars(v.mine.stars); setComment(v.mine.comment) }
    }).catch(() => setErr(t('product.reviewsLoadError')))
  }
  useEffect(load, [slug, token])

  async function onSubmit() {
    if (!token || stars < 1 || busy) return
    setBusy(true); setErr('')
    try {
      const v = await submitRating(token, slug, { stars, comment: comment.trim() })
      setView(v); if (v.mine) { setStars(v.mine.stars); setComment(v.mine.comment) }
    } catch (e) {
      setErr(e instanceof Error ? e.message : t('product.reviewsSubmitError'))
    } finally { setBusy(false) }
  }

  if (!view) return null
  return (
    <section className="reviews">
      <div className="rv-head">
        <h3>{t('product.reviewsHeading')}</h3>
        <span className="rv-summary">{view.count > 0 ? <><StarRow value={Math.round(view.average)} /> {view.average.toFixed(1)} · {t('product.reviewsCount', { count: view.count })}</> : t('catalog.notYetRated')}</span>
      </div>

      {token && view.canRate && (
        <div className="rv-form">
          <div className="rv-pick" role="group" aria-label={t('product.reviewsYourRating')}>
            {[1, 2, 3, 4, 5].map(n => (
              <button key={n} type="button" aria-label={t('product.reviewsRateStar', { n })}
                className={`rv-star ${n <= stars ? 'on' : ''}`} onClick={() => setStars(n)}>★</button>
            ))}
          </div>
          <textarea placeholder={t('product.reviewsCommentPlaceholder')} value={comment} maxLength={500}
            onChange={e => setComment(e.target.value)} />
          <button className="btn btn-primary" disabled={busy || stars < 1} onClick={onSubmit}>
            {view.mine ? t('product.reviewsUpdate') : t('product.reviewsPublish')}
          </button>
        </div>
      )}
      {token && !view.canRate && <p className="rv-note">{t('product.reviewsSubscribePrompt')}</p>}
      {!token && <p className="rv-note">{t('product.reviewsLoginPrompt')}</p>}
      {err && <p className="autherr" role="alert">{err}</p>}

      <ul className="rv-list">
        {view.items.map((rv, i) => (
          <li key={i} className="rv-item">
            <div className="rv-meta"><StarRow value={rv.stars} /> <b>{rv.author}</b> <span className="rv-when">{formatRelative(rv.createdAt)}</span></div>
            {rv.comment && <p className="rv-comment">{rv.comment}</p>}
          </li>
        ))}
      </ul>
    </section>
  )
}
