import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import { LanguageProvider, useLang, useT } from './LanguageProvider'
import { translate } from './t'
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
    // active-lang hit
    expect(translate('en', 'test.hello', { name: 'Ada' })).toBe(en.test.hello.replace('{name}', 'Ada'))
    // an entirely unknown key resolves to the key string itself (never blank/undefined)
    expect(translate('en', 'does.not.exist')).toBe('does.not.exist')
    expect(translate('fr', 'does.not.exist')).toBe('does.not.exist')
    // interpolation of an unknown var leaves the placeholder intact (never "undefined")
    expect(translate('en', 'test.hello', {})).toBe('Hello {name}')
  })
})
