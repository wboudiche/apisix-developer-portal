export interface Product {
  id: number
  name: string
  slug: string
  category: string
  version: string
  contextPath: string
  description: string
  tags: string[]
  icon: string
  rating: number
  ratingCount: number
  authType?: string
  lifecycleStatus?: 'active' | 'deprecated' | 'sunset'
  sunsetDate?: string | null
}

export interface Review { stars: number; comment: string; author: string; createdAt: string }
export interface RatingsView { average: number; count: number; items: Review[]; mine: Review | null; canRate: boolean }

export interface User {
  id: number
  email: string
  name: string
  role: string
  language?: 'fr' | 'en'
}

export interface AuthResponse {
  user: User
  token: string
}

export interface ProductQuery {
  search?: string
  category?: string
  tag?: string
  sort?: 'alpha' | 'rating'
}

export interface Plan {
  id: number
  name: string
  rateLimit: number
  windowSeconds: number
  priceCents: number
  currency: string
}

export interface Invoice {
  id: number
  teamId: number
  subscriptionId: number | null
  planName: string
  priceCents: number
  currency: string
  status: 'pending' | 'paid' | 'void'
  createdAt: string
  paidAt: string | null
}

export interface Application {
  id: number
  ownerId: number
  name: string
  description: string
  createdAt: string
  // Populated by the list endpoint (GET /api/applications) so the index can show
  // each app's subscription count without an extra fetch. Absent on create responses.
  subscriptionCount?: number
  teamId?: number
  teamName?: string
}

export interface Team {
  id: number
  name: string
  personal: boolean
  role: 'owner' | 'member'
  memberCount: number
}

export interface TeamMember {
  userId: number
  email: string
  name: string
  role: 'owner' | 'member'
}

export interface Credential {
  applicationId: number
  apiKey: string
  consumerUsername: string
}

export interface SubscriptionView {
  productId: number
  productName: string
  version: string
  contextPath: string
  planId: number
  planName: string
  status: string
  sandboxAvailable?: boolean
}

export interface AdminProduct {
  id?: number
  name: string
  slug: string
  category: string
  version: string
  contextPath: string
  description: string
  tags: string[]
  icon: string
  upstreamUrl: string
  published: boolean
  openapiSpec?: string
  sandboxUpstreamUrl?: string
  authType?: string
  lifecycleStatus?: 'active' | 'deprecated' | 'sunset'
  sunsetDate?: string | null
}

export interface ChangelogEntry {
  id: number
  version: string
  kind: string
  notes: string
  date: string
}

export interface AdminSubscription {
  id: number
  applicationName: string
  ownerEmail: string
  productName: string
  version: string
  planName: string
  status: string
  createdAt: string
}

export type AppEventKind =
  | 'app_created'
  | 'subscribed'
  | 'approved'
  | 'rejected'
  | 'unsubscribed'
  | 'key_rotated'
  | 'sandbox_enabled'
  | 'sandbox_key_rotated'

export interface AppEvent {
  kind: AppEventKind
  productName: string
  planName: string
  createdAt: string
}

export interface AppDetail {
  apiKey: string
  consumerUsername: string
  sandboxEnabled?: boolean
  sandboxGatewayUrl?: string
  oidcClientId?: string
  oauthEligible?: boolean
  oidcIssuer?: string
  subscriptions: SubscriptionView[]
  events: AppEvent[]
}

// Usage metrics for the Overview cards and the traffic chart, served by
// GET /api/applications/{id}/usage. Range is an allow-listed enum on the API.
export type UsageRange = '24h' | '7d' | '30d'

export interface UsageSummary {
  requestsToday: number
  monthToDate: number
  p95Ms: number
  errorRate: number // 0..1
}

export interface UsagePoint {
  t: string // RFC3339 bucket timestamp
  requests: number
  errors: number
}

export interface Usage {
  summary: UsageSummary
  series: UsagePoint[]
}

// Envelope returned by every paginated list endpoint.
export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  pageSize: number
}

export type TryApp = { id: number; name: string }

export interface Quota {
  hasQuota: boolean
  used?: number
  limit?: number
  windowSeconds?: number
  available?: boolean
}
