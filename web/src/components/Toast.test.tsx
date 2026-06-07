import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Toast } from './Toast'

describe('Toast', () => {
  it('shows the message with the ok icon by default', () => {
    render(<Toast msg="Créé" />)
    const el = screen.getByRole('status')
    expect(el).toHaveTextContent('Créé')
    expect(el.className).toContain('show')
    expect(el.className).not.toContain('warn')
  })
  it('renders the warn variant', () => {
    render(<Toast msg="Supprimé" kind="warn" />)
    expect(screen.getByRole('status').className).toContain('warn')
  })
  it('stays hidden with a null message', () => {
    render(<Toast msg={null} />)
    expect(screen.getByRole('status').className).not.toContain('show')
  })
})
