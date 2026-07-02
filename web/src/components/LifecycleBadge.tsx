export function LifecycleBadge({ status }: { status?: string }) {
  if (status === 'deprecated') return <span className="pill lifecycle deprecated">Déprécié</span>
  if (status === 'sunset') return <span className="pill lifecycle sunset">Sunset</span>
  return null
}
