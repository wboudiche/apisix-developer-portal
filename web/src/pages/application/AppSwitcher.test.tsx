import { useState, type ReactElement } from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AppSwitcher, CreateAppModal } from './AppSwitcher'
import { LanguageProvider } from '../../i18n/LanguageProvider'
import type { Application } from '../../api/types'

const apps: Application[] = [
  { id: 1, ownerId: 1, name: 'Boutique Mobile', description: '', createdAt: '2026-03-12T00:00:00Z' },
  { id: 2, ownerId: 1, name: 'Analytics interne', description: '', createdAt: '2026-04-02T00:00:00Z' },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
})

function renderSwitcher(ui: ReactElement) {
  return render(<LanguageProvider><MemoryRouter>{ui}</MemoryRouter></LanguageProvider>)
}
function renderModal(ui: ReactElement) {
  return render(<LanguageProvider>{ui}</LanguageProvider>)
}

describe('AppSwitcher', () => {
  it('opens the menu listing all apps, current one marked', async () => {
    renderSwitcher(<AppSwitcher apps={apps} currentId={1} onCreate={() => {}} />)
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    expect(screen.getByText('Analytics interne')).toBeInTheDocument()
    expect(screen.getByText('Boutique Mobile').closest('a')).toHaveClass('cur')
    expect(screen.getByText('app_2')).toBeInTheDocument()
  })
  it('exposes the Nouvelle application action as a real button', async () => {
    const onCreate = vi.fn()
    renderSwitcher(<AppSwitcher apps={apps} currentId={1} onCreate={onCreate} />)
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    const item = screen.getByText('Nouvelle application').closest('button')
    expect(item).not.toBeNull() // keyboard-reachable, fires on Enter/Space
    await userEvent.click(item!)
    expect(onCreate).toHaveBeenCalledTimes(1)
  })
})

describe('CreateAppModal', () => {
  it('creates with the typed name', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    renderModal(<CreateAppModal open onClose={() => {}} onCreate={onCreate} />)
    await userEvent.type(screen.getByLabelText("Nom de l'application"), 'Mon App')
    await userEvent.click(screen.getByRole('button', { name: 'Créer' }))
    expect(onCreate).toHaveBeenCalledWith('Mon App')
  })
  it('renders nothing when closed', () => {
    const { container } = renderModal(<CreateAppModal open={false} onClose={() => {}} onCreate={async () => {}} />)
    expect(container.firstChild).toBeNull()
  })
  it('is an accessible dialog: Annuler and Escape close without creating', async () => {
    const onClose = vi.fn()
    const onCreate = vi.fn().mockResolvedValue(undefined)
    renderModal(<CreateAppModal open onClose={onClose} onCreate={onCreate} />)
    expect(screen.getByRole('dialog', { name: 'Nouvelle application' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Annuler' }))
    expect(onClose).toHaveBeenCalledTimes(1)
    await userEvent.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(2)
    expect(onCreate).not.toHaveBeenCalled()
  })
  it('restores focus to the trigger when closed', async () => {
    function Host() {
      const [open, setOpen] = useState(false)
      return (
        <>
          <button onClick={() => setOpen(true)}>ouvrir</button>
          <CreateAppModal open={open} onClose={() => setOpen(false)} onCreate={async () => {}} />
        </>
      )
    }
    renderModal(<Host />)
    const trigger = screen.getByRole('button', { name: 'ouvrir' })
    await userEvent.click(trigger)
    expect(document.activeElement).toBe(screen.getByLabelText("Nom de l'application"))
    await userEvent.click(screen.getByRole('button', { name: 'Annuler' }))
    expect(document.activeElement).toBe(trigger)
  })
})
