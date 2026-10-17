import type { CollectorStatus, NowPlaying, Play, PlaysPage, Summary } from '../api/types'

export const collector = (over: Partial<CollectorStatus> = {}): CollectorStatus => ({
  state: 'ok',
  stale: false,
  fetch_mode: 'direct',
  poll_interval_seconds: 60,
  stale_after_seconds: 180,
  first_success_at: '2026-10-01T00:00:00Z',
  last_attempt_at: new Date().toISOString(),
  last_success_at: new Date().toISOString(),
  next_attempt_at: null,
  last_error_at: null,
  consecutive_failures: 0,
  ...over,
})

export const play = (over: Partial<Play> = {}): Play => ({
  id: 1,
  track: {
    id: 10,
    title: 'Sustoja Laikas (Meile)',
    kind: 'music',
    artist: { id: 20, name: 'Leon Somov & Kaia' },
  },
  start_at: '2026-10-17T16:00:10Z',
  start_authoritative: true,
  upstream_start_at: '2026-10-17T16:00:10Z',
  first_observed_at: '2026-10-17T16:00:43Z',
  last_observed_at: '2026-10-17T16:03:45Z',
  observation_count: 7,
  reason: 'upstream_event',
  uncertain: false,
  ...over,
})

export const nowPlaying = (over: Partial<NowPlaying> = {}): NowPlaying => ({
  play: play(),
  collector: collector(),
  timezone: 'Europe/Vilnius',
  ...over,
})

export const playsPage = (items: Play[], next_cursor: string | null = null): PlaysPage => ({
  items,
  next_cursor,
})

export const summary = (plays = 0): Summary => ({
  range: {
    from: '2026-10-11T00:00:00+03:00',
    to: '2026-10-18T00:00:00+03:00',
    timezone: 'Europe/Vilnius',
  },
  totals: {
    plays,
    uncertain_plays: 0,
    distinct_tracks: plays ? 1 : 0,
    distinct_artists: plays ? 1 : 0,
  },
  comparison: {
    current: {
      from: '2026-10-11T00:00:00+03:00',
      to: '2026-10-17T19:00:00+03:00',
      timezone: 'Europe/Vilnius',
    },
    previous: {
      from: '2026-10-04T05:00:00+03:00',
      to: '2026-10-11T00:00:00+03:00',
      timezone: 'Europe/Vilnius',
    },
    current_plays: plays,
    change: { previous: 0, delta: plays, state: plays ? 'new' : 'none', percent: null },
    current_closed: false,
  },
  coverage: {
    collection_started_at: '2026-10-01T00:00:00Z',
    observations_since: '2026-10-01T00:00:00Z',
    gaps: [],
    gaps_truncated: false,
    partial: false,
    min_gap_seconds: 300,
    collector: collector(),
  },
  semantics: {},
})
