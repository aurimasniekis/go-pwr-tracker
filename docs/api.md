# HTTP API

Read-only JSON over `GET`. Base path `/api/v1`. Same origin as the UI; no
CORS. Types are mirrored in `web/src/api/types.ts`.

## Conventions

- **Timestamps**: RFC 3339. Stored instants are returned in UTC (`Z`); range
  boundaries and buckets are returned in the analytics timezone with an offset.
- **Ranges**: `from` / `to` accept RFC 3339 instants or `YYYY-MM-DD` dates
  (local midnight in the analytics timezone). Ranges are half-open
  `[from, to)`. Analytics default to the last 7 local calendar days including
  today; the maximum span is 400 days.
- **Errors**: `{"error": {"code": "...", "message": "..."}}` with codes
  `invalid_parameter`, `invalid_range` (400), `not_found` (404), `internal`
  (500). Unknown `/api/...` paths return a JSON 404, never the SPA.
- **Bounds**: `limit` 1–100, `offset` 0–10000, `q` ≤ 100 bytes. IDs are
  positive integers.
- **Search** (`q`): case-insensitive (Unicode case folding) substring of the
  track title or artist name.

## Endpoints

| Endpoint | Parameters | Returns |
|---|---|---|
| `GET /healthz` | – | Liveness: `200 {"status":"ok"}` while the process runs |
| `GET /readyz` | – | Readiness: `200` when the database answers, else `503`. Ignores upstream state |
| `GET /api/v1/now-playing` | – | Latest play (kept across failures), collector status, timezone |
| `GET /api/v1/status` | – | Collector status, continuity gap, schema version, build info |
| `GET /api/v1/plays` | `limit` (25), `cursor`, `from`, `to`, `track_id`, `artist_id`, `q`, `include_segments` | Activity feed newest first + `next_cursor` |
| `GET /api/v1/tracks` | `q`, `artist_id`, `limit`, `offset` | Music tracks, most played first |
| `GET /api/v1/tracks/{id}` | – | Track with totals, first/last observed |
| `GET /api/v1/artists` | `q`, `limit`, `offset` | Artists, most played first |
| `GET /api/v1/artists/{id}` | – | Artist with totals |
| `GET /api/v1/stats/summary` | `from`, `to`, `track_id`, `artist_id` | Totals, comparison, coverage/gaps |
| `GET /api/v1/stats/top-tracks` | `from`, `to`, `artist_id`, `limit` (10) | Ranked tracks with per-item change |
| `GET /api/v1/stats/top-artists` | `from`, `to`, `limit` (10) | Ranked artists with per-item change |
| `GET /api/v1/stats/airplay` | `from`, `to`, `bucket`, `track_id`, `artist_id` | Time series |
| `GET /api/v1/stats/hourly` | `from`, `to`, `track_id`, `artist_id` | Hour-of-day and weekday×hour counts |

### Collector status

`state` is one of `starting` (no success yet), `ok`, `error` (last attempt
failed, data still fresh), `stale` (no success within `STALE_AFTER`),
`disabled`. `stale` is a separate boolean. `last_error` is present only while
failures are ongoing and is prefixed with a kind: `upstream_status`, `solver`,
`invalid_payload`, `too_large`, `timeout`, `network`.

### Activity feed pagination

Ordered by `(start_at DESC, id DESC)`. `next_cursor` is an opaque token
encoding the last row's `(start_at, id)`; pass it back as `cursor` with the
same filters. New plays arriving between requests never cause skips or
duplicates in later pages. Station segments are excluded unless
`include_segments=true`.

### Buckets (`stats/airplay`)

`bucket` is `hour`, `day` or `week` (Monday start); default chosen by span
(≤ 3 days: hour, ≤ 120 days: day, else week). Hourly buckets are limited to
31-day spans. Buckets are aligned to local boundaries in the analytics
timezone. Each point has `start`, `end`, `plays`, `uncertain_plays` and
`partial` — true when the bucket extends outside the range, has not fully
elapsed, or starts before collection began.

### Comparisons

The current period is clipped to now; the previous period is the immediately
preceding period of the same length. `change.state`:

| state | meaning | `percent` |
|---|---|---|
| `up` / `down` / `flat` | both periods have data | rounded to 0.1 |
| `new` | previous period had 0 plays | `null` |
| `none` | 0 in both | `null` |
| `unavailable` | collection had not started by the start of the previous period | `null` |

### Coverage (`stats/summary`)

`coverage.gaps` lists periods longer than `CONTINUITY_GAP` without a
successful observation (from retained observations, plus an open trailing gap
if the collector is currently failing). `observations_since` is the oldest
retained observation; gaps before it cannot be detected. `partial` is true
when there are gaps, collection started after `from`, or `to` is in the
future.
