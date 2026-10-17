import type { QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  type RouterHistory,
} from '@tanstack/react-router'
import { RootLayout, NotFound } from './routes/RootLayout'
import { Dashboard } from './routes/Dashboard'
import { Activity } from './routes/Activity'
import { TrackDetail } from './routes/TrackDetail'
import { ArtistDetail } from './routes/ArtistDetail'
import { Search } from './routes/Search'
import { isRangePreset, type RangePreset } from './lib/range'

// Search params are validated here so filters are typed and shareable.

export interface RangeSearch {
  range?: RangePreset
}

export interface ActivitySearch {
  q?: string
  from?: string
  to?: string
  track_id?: number
  artist_id?: number
}

const isoDate = /^\d{4}-\d{2}-\d{2}$/

function str(v: unknown, max = 100): string | undefined {
  return typeof v === 'string' && v.trim() !== '' ? v.slice(0, max) : undefined
}

function posInt(v: unknown): number | undefined {
  const n = typeof v === 'number' ? v : typeof v === 'string' ? Number(v) : NaN
  return Number.isInteger(n) && n > 0 ? n : undefined
}

function date(v: unknown): string | undefined {
  return typeof v === 'string' && isoDate.test(v) ? v : undefined
}

const validateRange = (s: Record<string, unknown>): RangeSearch => ({
  range: isRangePreset(s.range) ? s.range : undefined,
})

const rootRoute = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: RootLayout,
  notFoundComponent: NotFound,
})

const dashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  validateSearch: validateRange,
  component: Dashboard,
})

const activityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/activity',
  validateSearch: (s: Record<string, unknown>): ActivitySearch => ({
    q: str(s.q),
    from: date(s.from),
    to: date(s.to),
    track_id: posInt(s.track_id),
    artist_id: posInt(s.artist_id),
  }),
  component: Activity,
})

const trackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/tracks/$trackId',
  params: {
    parse: (p) => ({ trackId: posInt(p.trackId) ?? 0 }),
    stringify: (p) => ({ trackId: String(p.trackId) }),
  },
  validateSearch: validateRange,
  component: TrackDetail,
})

const artistRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/artists/$artistId',
  params: {
    parse: (p) => ({ artistId: posInt(p.artistId) ?? 0 }),
    stringify: (p) => ({ artistId: String(p.artistId) }),
  },
  validateSearch: validateRange,
  component: ArtistDetail,
})

const searchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/search',
  validateSearch: (s: Record<string, unknown>): { q?: string } => ({ q: str(s.q) }),
  component: Search,
})

const routeTree = rootRoute.addChildren([
  dashboardRoute,
  activityRoute,
  trackRoute,
  artistRoute,
  searchRoute,
])

export function makeRouter(queryClient: QueryClient, history?: RouterHistory) {
  return createRouter({
    routeTree,
    history,
    context: { queryClient },
    defaultPreload: 'intent',
    scrollRestoration: true,
  })
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof makeRouter>
  }
}
