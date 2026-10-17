import {
  infiniteQueryOptions,
  keepPreviousData,
  queryOptions,
  useQuery,
} from '@tanstack/react-query'
import { api, type PlaysParams, type StatsParams } from './client'
import type { Bucket } from './types'

// Freshness is aligned with the collector: now-playing refreshes at half the
// poll interval (bounded to 15–60 s); feeds once a minute; analytics on
// focus/navigation only. TanStack Query pauses interval refetching while the
// tab is hidden (refetchIntervalInBackground defaults to false).

export const nowPlayingQuery = queryOptions({
  queryKey: ['now-playing'],
  queryFn: ({ signal }) => api.nowPlaying(signal),
  refetchInterval: (q) => {
    const secs = q.state.data?.collector.poll_interval_seconds ?? 60
    return Math.min(60, Math.max(15, secs / 2)) * 1000
  },
})

export function useNowPlaying() {
  return useQuery(nowPlayingQuery)
}

/** The station's analytics timezone, as reported by the API. */
export function useStationTimeZone(): string {
  const { data } = useQuery({ ...nowPlayingQuery, select: (d) => d.timezone })
  return data ?? 'Europe/Vilnius'
}

export const recentPlaysQuery = (limit: number) =>
  queryOptions({
    queryKey: ['plays', 'recent', limit],
    queryFn: ({ signal }) => api.plays({ limit }, signal),
    refetchInterval: 60_000,
  })

export const playsFeedQuery = (filters: Omit<PlaysParams, 'cursor'>) =>
  infiniteQueryOptions({
    queryKey: ['plays', 'feed', filters],
    queryFn: ({ pageParam, signal }) =>
      api.plays({ ...filters, cursor: pageParam ?? undefined }, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.next_cursor,
    // Only the first page is refreshed periodically; older pages are stable.
    refetchInterval: 60_000,
    placeholderData: keepPreviousData,
  })

const STATS_STALE = 60_000

export const summaryQuery = (p: StatsParams) =>
  queryOptions({
    queryKey: ['stats', 'summary', p],
    queryFn: ({ signal }) => api.summary(p, signal),
    staleTime: STATS_STALE,
    placeholderData: keepPreviousData,
  })

export const topTracksQuery = (p: StatsParams & { limit?: number }) =>
  queryOptions({
    queryKey: ['stats', 'top-tracks', p],
    queryFn: ({ signal }) => api.topTracks(p, signal),
    staleTime: STATS_STALE,
    placeholderData: keepPreviousData,
  })

export const topArtistsQuery = (p: { from?: string; to?: string; limit?: number }) =>
  queryOptions({
    queryKey: ['stats', 'top-artists', p],
    queryFn: ({ signal }) => api.topArtists(p, signal),
    staleTime: STATS_STALE,
    placeholderData: keepPreviousData,
  })

export const airplayQuery = (p: StatsParams & { bucket?: Bucket }) =>
  queryOptions({
    queryKey: ['stats', 'airplay', p],
    queryFn: ({ signal }) => api.airplay(p, signal),
    staleTime: STATS_STALE,
    placeholderData: keepPreviousData,
  })

export const hourlyQuery = (p: StatsParams) =>
  queryOptions({
    queryKey: ['stats', 'hourly', p],
    queryFn: ({ signal }) => api.hourly(p, signal),
    staleTime: STATS_STALE,
    placeholderData: keepPreviousData,
  })

export const trackQuery = (id: number) =>
  queryOptions({
    queryKey: ['track', id],
    queryFn: ({ signal }) => api.track(id, signal),
    staleTime: STATS_STALE,
  })

export const artistQuery = (id: number) =>
  queryOptions({
    queryKey: ['artist', id],
    queryFn: ({ signal }) => api.artist(id, signal),
    staleTime: STATS_STALE,
  })

export const searchQuery = (q: string) =>
  queryOptions({
    queryKey: ['search', q],
    queryFn: async ({ signal }) => {
      const [tracks, artists] = await Promise.all([
        api.tracks({ q, limit: 25 }, signal),
        api.artists({ q, limit: 25 }, signal),
      ])
      return { tracks: tracks.items, artists: artists.items }
    },
    staleTime: STATS_STALE,
    placeholderData: keepPreviousData,
  })

export const recentPlaysForQuery = (p: { track_id?: number; artist_id?: number }) =>
  queryOptions({
    queryKey: ['plays', 'recent-for', p],
    queryFn: ({ signal }) => api.plays({ ...p, limit: 15 }, signal),
    staleTime: STATS_STALE,
  })
