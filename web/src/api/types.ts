// Types mirroring the Go JSON API (internal/api, internal/store,
// internal/analytics). Timestamps are RFC 3339 strings. Keep in sync with
// the Go structs; the API contract is documented in docs/api.md.

export type Timestamp = string

export interface ArtistRef {
  id: number
  name: string
}

export type TrackKind = 'music' | 'segment'

export interface TrackRef {
  id: number
  title: string
  kind: TrackKind
  artist: ArtistRef | null
}

export type PlayReason =
  'upstream_event' | 'observed_transition' | 'initial_observation' | 'continuity_gap'

export interface Play {
  id: number
  track: TrackRef
  start_at: Timestamp
  start_authoritative: boolean
  upstream_start_at: Timestamp | null
  first_observed_at: Timestamp
  last_observed_at: Timestamp
  observation_count: number
  reason: PlayReason
  uncertain: boolean
}

export type CollectorState = 'disabled' | 'starting' | 'ok' | 'stale' | 'error'

export interface CollectorStatus {
  state: CollectorState
  stale: boolean
  fetch_mode: string
  poll_interval_seconds: number
  stale_after_seconds: number
  first_success_at: Timestamp | null
  last_attempt_at: Timestamp | null
  last_success_at: Timestamp | null
  next_attempt_at: Timestamp | null
  last_error?: string
  last_error_at: Timestamp | null
  consecutive_failures: number
}

export interface NowPlaying {
  play: Play | null
  collector: CollectorStatus
  timezone: string
}

export interface PlaysPage {
  items: Play[]
  next_cursor: string | null
}

export interface TrackSummary extends TrackRef {
  first_observed_at: Timestamp
  last_observed_at: Timestamp
  total_plays: number
  uncertain_plays: number
  last_play_at: Timestamp | null
}

export interface ArtistSummary extends ArtistRef {
  track_count: number
  total_plays: number
  uncertain_plays: number
  first_observed_at: Timestamp | null
  last_observed_at: Timestamp | null
  last_play_at: Timestamp | null
}

export interface ListResponse<T> {
  items: T[]
}

export interface RangeInfo {
  from: Timestamp
  to: Timestamp
  timezone: string
}

export type ChangeState = 'up' | 'down' | 'flat' | 'new' | 'none' | 'unavailable'

export interface Change {
  previous: number
  delta: number
  state: ChangeState
  percent: number | null
}

export interface Totals {
  plays: number
  uncertain_plays: number
  distinct_tracks: number
  distinct_artists: number
}

export interface Gap {
  from: Timestamp
  to: Timestamp
}

export interface Summary {
  range: RangeInfo
  totals: Totals
  comparison: {
    current: RangeInfo
    previous: RangeInfo
    current_plays: number
    change: Change
    current_closed: boolean
  } | null
  coverage: {
    collection_started_at: Timestamp | null
    observations_since: Timestamp | null
    gaps: Gap[]
    gaps_truncated: boolean
    partial: boolean
    min_gap_seconds: number
    collector: CollectorStatus
  }
  semantics: Record<string, string>
}

export interface Ranking<T> {
  range: RangeInfo
  previous: RangeInfo | null
  items: T[]
}

export interface TrackCount {
  track: TrackRef
  plays: number
  uncertain_plays: number
  change: Change | null
}

export interface ArtistCount {
  artist: ArtistRef
  plays: number
  uncertain_plays: number
  distinct_tracks: number
  change: Change | null
}

export type Bucket = 'hour' | 'day' | 'week'

export interface SeriesPoint {
  start: Timestamp
  end: Timestamp
  plays: number
  uncertain_plays: number
  partial: boolean
}

export interface Airplay {
  range: RangeInfo
  bucket: Bucket
  points: SeriesPoint[]
}

export interface Hourly {
  range: RangeInfo
  plays: number
  hours: number[]
  weekday_hours: number[][]
}

export interface ApiErrorBody {
  error: { code: string; message: string }
}
