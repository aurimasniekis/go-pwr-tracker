import type {
  Airplay,
  ApiErrorBody,
  ArtistCount,
  ArtistSummary,
  Bucket,
  Hourly,
  ListResponse,
  NowPlaying,
  PlaysPage,
  Ranking,
  Summary,
  TrackCount,
  TrackSummary,
} from './types'

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

type Params = Record<string, string | number | boolean | undefined | null>

function buildUrl(path: string, params: Params = {}): string {
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
  }
  const q = qs.toString()
  return `/api/v1${path}${q ? `?${q}` : ''}`
}

export async function getJSON<T>(path: string, params?: Params, signal?: AbortSignal): Promise<T> {
  const res = await fetch(buildUrl(path, params), {
    signal,
    headers: { Accept: 'application/json' },
  })
  if (!res.ok) {
    let code = 'http_error'
    let message = `Request failed with HTTP ${res.status}`
    try {
      const body = (await res.json()) as ApiErrorBody
      code = body.error.code
      message = body.error.message
    } catch {
      // non-JSON error body (e.g. proxy error page)
    }
    throw new ApiError(res.status, code, message)
  }
  return (await res.json()) as T
}

export interface RangeParams {
  from?: string
  to?: string
}

export interface PlaysParams extends RangeParams {
  cursor?: string
  limit?: number
  q?: string
  track_id?: number
  artist_id?: number
}

export interface StatsParams extends RangeParams {
  track_id?: number
  artist_id?: number
}

export const api = {
  nowPlaying: (signal?: AbortSignal) => getJSON<NowPlaying>('/now-playing', {}, signal),
  plays: (p: PlaysParams, signal?: AbortSignal) => getJSON<PlaysPage>('/plays', { ...p }, signal),
  tracks: (p: { q?: string; artist_id?: number; limit?: number }, signal?: AbortSignal) =>
    getJSON<ListResponse<TrackSummary>>('/tracks', p, signal),
  track: (id: number, signal?: AbortSignal) => getJSON<TrackSummary>(`/tracks/${id}`, {}, signal),
  artists: (p: { q?: string; limit?: number }, signal?: AbortSignal) =>
    getJSON<ListResponse<ArtistSummary>>('/artists', p, signal),
  artist: (id: number, signal?: AbortSignal) =>
    getJSON<ArtistSummary>(`/artists/${id}`, {}, signal),
  summary: (p: StatsParams, signal?: AbortSignal) =>
    getJSON<Summary>('/stats/summary', { ...p }, signal),
  topTracks: (p: StatsParams & { limit?: number }, signal?: AbortSignal) =>
    getJSON<Ranking<TrackCount>>('/stats/top-tracks', { ...p }, signal),
  topArtists: (p: RangeParams & { limit?: number }, signal?: AbortSignal) =>
    getJSON<Ranking<ArtistCount>>('/stats/top-artists', { ...p }, signal),
  airplay: (p: StatsParams & { bucket?: Bucket }, signal?: AbortSignal) =>
    getJSON<Airplay>('/stats/airplay', { ...p }, signal),
  hourly: (p: StatsParams, signal?: AbortSignal) =>
    getJSON<Hourly>('/stats/hourly', { ...p }, signal),
}
