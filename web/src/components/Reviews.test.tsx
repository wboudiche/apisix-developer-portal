import { it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Reviews } from './Reviews'
import { LanguageProvider } from '../i18n/LanguageProvider'
import * as api from '../api/client'

beforeEach(() => {
  localStorage.clear()
  // jsdom's navigator.language defaults to 'en-US', which would auto-detect to
  // English; force French so existing assertions (against French strings) hold.
  localStorage.setItem('lang', 'fr')
})
afterEach(() => vi.restoreAllMocks())

function renderReviews(props: { slug: string; token: string | null }) {
  return render(<LanguageProvider><Reviews {...props} /></LanguageProvider>)
}

it('renders the summary and review list', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({
    average: 4.5, count: 2, canRate: false, mine: null,
    items: [{ stars: 5, comment: 'top', author: 'Alice', createdAt: '2026-06-01T00:00:00Z' }],
  })
  renderReviews({ slug: 'orders', token: null })
  expect(await screen.findByText(/2 avis/)).toBeInTheDocument()
  expect(screen.getByText('top')).toBeInTheDocument()
  expect(screen.getByText('Alice')).toBeInTheDocument()
  // no form for a non-subscriber
  expect(screen.queryByRole('button', { name: /Publier|Envoyer|Noter/i })).not.toBeInTheDocument()
})

it('lets an approved subscriber submit a rating', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({ average: 0, count: 0, canRate: true, mine: null, items: [] })
  const submit = vi.spyOn(api, 'submitRating').mockResolvedValue({ average: 5, count: 1, canRate: true, mine: { stars: 5, comment: 'super', author: 'Me', createdAt: '' }, items: [{ stars: 5, comment: 'super', author: 'Me', createdAt: '' }] })
  renderReviews({ slug: 'orders', token: 'jwt' })
  // pick 5 stars (the form exposes star buttons labelled "Noter N étoiles")
  await userEvent.click(await screen.findByRole('button', { name: /Noter 5/i }))
  await userEvent.type(screen.getByPlaceholderText(/commentaire/i), 'super')
  await userEvent.click(screen.getByRole('button', { name: /Publier|Envoyer/i }))
  await waitFor(() => expect(submit).toHaveBeenCalledWith('jwt', 'orders', { stars: 5, comment: 'super' }))
})

it('shows a subscribe prompt when authed but cannot rate', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({ average: 0, count: 0, canRate: false, mine: null, items: [] })
  renderReviews({ slug: 'orders', token: 'jwt' })
  expect(await screen.findByText(/Abonnez-vous pour noter/i)).toBeInTheDocument()
})
