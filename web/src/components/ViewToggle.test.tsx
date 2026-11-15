import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { LanguageProvider } from '../i18n/LanguageProvider'
import { ViewToggle } from './ViewToggle'
import { usePersistedView } from './usePersistedView'

function Harness({ storageKey, fallback }: { storageKey: string; fallback: 'list' | 'cards' }) {
  const [view, pick] = usePersistedView(storageKey, fallback)
  return <><ViewToggle value={view} onChange={pick} /><output>{view}</output></>
}
const renderHarness = (fallback: 'list' | 'cards' = 'list') =>
  render(<LanguageProvider><Harness storageKey="testView" fallback={fallback} /></LanguageProvider>)

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('lang', 'fr')
})

describe('ViewToggle + usePersistedView', () => {
  it('exposes a named group with two pressed-state buttons', () => {
    renderHarness()
    expect(screen.getByRole('group', { name: "Mode d'affichage" })).toBeInTheDocument()
    expect(screen.getByLabelText('Vue liste')).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByLabelText('Vue grille')).toHaveAttribute('aria-pressed', 'false')
  })

  it('switches view and remembers the choice', () => {
    renderHarness()
    fireEvent.click(screen.getByLabelText('Vue grille'))
    expect(screen.getByText('cards')).toBeInTheDocument()
    expect(localStorage.getItem('testView')).toBe('cards')
  })

  it('restores a stored choice and ignores unknown stored values', () => {
    localStorage.setItem('testView', 'cards')
    const { unmount } = renderHarness('list')
    expect(screen.getByText('cards')).toBeInTheDocument()
    unmount()
    localStorage.setItem('testView', 'bogus')
    renderHarness('list')
    expect(screen.getByText('list')).toBeInTheDocument()
  })
})
