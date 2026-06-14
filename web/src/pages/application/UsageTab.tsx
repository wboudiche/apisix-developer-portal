import { useState } from 'react'
import type { UsageRange } from '../../api/types'
import { useUsage } from './useUsage'
import { UsageChart } from './UsageChart'

// UsageTab shows the real traffic chart with a selectable range. The per-product
// "Répartition par API" breakdown is deferred — the /usage endpoint is
// app-level; per-product rows need route→product attribution (see the plan doc).
export function UsageTab({ token, appId }: { token: string; appId: number }) {
  const [range, setRange] = useState<UsageRange>('7d')
  const usage = useUsage(token, appId, range)
  return (
    <section className="panel">
      <UsageChart state={usage} range={range} onRange={setRange} />
    </section>
  )
}
