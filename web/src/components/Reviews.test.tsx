import { it, expect, vi, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Reviews } from './Reviews'
import * as api from '../api/client'

afterEach(() => vi.restoreAllMocks())

it('renders the summary and review list', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({
    average: 4.5, count: 2, canRate: false, mine: null,
    items: [{ stars: 5, comment: 'top', author: 'Alice', createdAt: '2026-06-01T00:00:00Z' }],
  })
  render(<Reviews slug="orders" token={null} />)
  expect(await screen.findByText(/2 avis/)).toBeInTheDocument()
  expect(screen.getByText('top')).toBeInTheDocument()
  expect(screen.getByText('Alice')).toBeInTheDocument()
  // no form for a non-subscriber
  expect(screen.queryByRole('button', { name: /Publier|Envoyer|Noter/i })).not.toBeInTheDocument()
})

it('lets an approved subscriber submit a rating', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({ average: 0, count: 0, canRate: true, mine: null, items: [] })
  const submit = vi.spyOn(api, 'submitRating').mockResolvedValue({ average: 5, count: 1, canRate: true, mine: { stars: 5, comment: 'super', author: 'Me', createdAt: '' }, items: [{ stars: 5, comment: 'super', author: 'Me', createdAt: '' }] })
  render(<Reviews slug="orders" token="jwt" />)
  // pick 5 stars (the form exposes star buttons labelled "Noter N étoiles")
  await userEvent.click(await screen.findByRole('button', { name: /Noter 5/i }))
  await userEvent.type(screen.getByPlaceholderText(/commentaire/i), 'super')
  await userEvent.click(screen.getByRole('button', { name: /Publier|Envoyer/i }))
  await waitFor(() => expect(submit).toHaveBeenCalledWith('jwt', 'orders', { stars: 5, comment: 'super' }))
})

it('shows a subscribe prompt when authed but cannot rate', async () => {
  vi.spyOn(api, 'getRatings').mockResolvedValue({ average: 0, count: 0, canRate: false, mine: null, items: [] })
  render(<Reviews slug="orders" token="jwt" />)
  expect(await screen.findByText(/Abonnez-vous pour noter/i)).toBeInTheDocument()
})
