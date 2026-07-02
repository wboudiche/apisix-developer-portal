import { useT } from '../i18n/LanguageProvider'

export function LifecycleBadge({ status }: { status?: string }) {
  const t = useT()
  if (status === 'deprecated') return <span className="pill lifecycle deprecated">{t('lifecycleBadge.deprecated')}</span>
  if (status === 'sunset') return <span className="pill lifecycle sunset">Sunset</span>
  return null
}
