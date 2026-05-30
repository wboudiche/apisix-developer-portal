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
}

export interface User {
  id: number
  email: string
  name: string
  role: string
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
}

export interface Application {
  id: number
  ownerId: number
  name: string
  description: string
  createdAt: string
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

export interface AppDetail {
  apiKey: string
  consumerUsername: string
  subscriptions: SubscriptionView[]
}
