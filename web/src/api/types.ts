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
