import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AppSwitcher, CreateAppModal } from './AppSwitcher'
import type { Application } from '../../api/types'

const apps: Application[] = [
  { id: 1, ownerId: 1, name: 'Boutique Mobile', description: '', createdAt: '2026-03-12T00:00:00Z' },
  { id: 2, ownerId: 1, name: 'Analytics interne', description: '', createdAt: '2026-04-02T00:00:00Z' },
]

describe('AppSwitcher', () => {
  it('opens the menu listing all apps, current one marked', async () => {
    render(<MemoryRouter><AppSwitcher apps={apps} currentId={1} onCreate={() => {}} /></MemoryRouter>)
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    expect(screen.getByText('Analytics interne')).toBeInTheDocument()
    expect(screen.getByText('Boutique Mobile').closest('a')).toHaveClass('cur')
    expect(screen.getByText('app_2')).toBeInTheDocument()
  })
  it('exposes the Nouvelle application action', async () => {
    const onCreate = vi.fn()
    render(<MemoryRouter><AppSwitcher apps={apps} currentId={1} onCreate={onCreate} /></MemoryRouter>)
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    await userEvent.click(screen.getByText('Nouvelle application'))
    expect(onCreate).toHaveBeenCalled()
  })
})

describe('CreateAppModal', () => {
  it('creates with the typed name', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(<CreateAppModal open onClose={() => {}} onCreate={onCreate} />)
    await userEvent.type(screen.getByLabelText("Nom de l'application"), 'Mon App')
    await userEvent.click(screen.getByRole('button', { name: 'Créer' }))
    expect(onCreate).toHaveBeenCalledWith('Mon App')
  })
  it('renders nothing when closed', () => {
    const { container } = render(<CreateAppModal open={false} onClose={() => {}} onCreate={async () => {}} />)
    expect(container.firstChild).toBeNull()
  })
})
