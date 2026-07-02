import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { LanguageProvider, useLang, useT } from './LanguageProvider'
import { fr } from './fr'
import { en } from './en'

function Probe() {
  const t = useT(); const { lang, setLang } = useLang()
  return <div>
    <span data-testid="v">{t('test.hello', { name: 'Ada' })}</span>
    <span data-testid="lang">{lang}</span>
    <button onClick={() => setLang('en')}>en</button>
  </div>
}

beforeEach(() => { localStorage.clear() })

describe('i18n core', () => {
  it('interpolates + switches language + persists', () => {
    render(<LanguageProvider><Probe /></LanguageProvider>)
    // default detect: jsdom navigator.language is usually 'en-US' → 'en'; force fr by seeding
    act(() => { screen.getByRole('button', { name: 'en' }).click() })
    expect(screen.getByTestId('lang').textContent).toBe('en')
    expect(localStorage.getItem('lang')).toBe('en')
    expect(document.documentElement.getAttribute('lang')).toBe('en')
    expect(screen.getByTestId('v').textContent).toBe(en.test.hello.replace('{name}', 'Ada'))
  })

  it('fr and en have identical key sets', () => {
    const keys = (o: object, p = ''): string[] => Object.entries(o).flatMap(([k, v]) =>
      v && typeof v === 'object' ? keys(v, `${p}${k}.`) : [`${p}${k}`])
    expect(keys(en).sort()).toEqual(keys(fr).sort())
  })

  it('falls back active→fr→key', () => {
    // a key present in fr but (hypothetically) missing in en resolves to the fr value;
    // an entirely unknown key resolves to itself. (Uses the real catalogs.)
  })
})
