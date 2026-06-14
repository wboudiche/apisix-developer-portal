import { useEffect, useRef, useState } from 'react'
import { getUsage } from '../../api/client'
import type { Usage, UsageRange } from '../../api/types'

// UsageState models the three things a usage panel can show: a loading skeleton,
// the data, or an explicit unavailable state. There is deliberately no demo
// fallback — a metrics outage must read as unavailable, not as real traffic.
export type UsageState =
  | { status: 'loading' }
  | { status: 'ready'; usage: Usage }
  | { status: 'error' }

// useUsage fetches usage asynchronously so the page shell never blocks on it.
// A monotonic guard ensures a slow response can't overwrite a newer range/app.
export function useUsage(token: string | null, appId: number, range: UsageRange): UsageState {
  const [state, setState] = useState<UsageState>({ status: 'loading' })
  const reqRef = useRef(0)

  useEffect(() => {
    if (!token || !Number.isFinite(appId)) return
    const seq = ++reqRef.current
    setState({ status: 'loading' })
    getUsage(token, appId, range)
      .then(u => { if (seq === reqRef.current) setState({ status: 'ready', usage: u }) })
      .catch(() => { if (seq === reqRef.current) setState({ status: 'error' }) })
    return () => { reqRef.current++ }
  }, [token, appId, range])

  return state
}
