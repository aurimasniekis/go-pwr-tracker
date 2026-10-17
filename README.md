# pwr-tracker

A small, self-hosted service that tracks what **Power Hit Radio** (Lithuania)
plays. It polls the station's now-playing endpoint about once a minute, turns
repeated observations into a deduplicated history of plays, and shows an
activity feed and airplay analytics in a web UI.

It answers:

- What is playing now, and is that information fresh?
- What played recently?
- Which tracks and artists get the most airplay?
- At which hours and on which weekdays are they played?
- How is their airplay changing compared with the previous period?

Rankings describe **airplay** (how often the station played something), not
listener popularity.

One Go binary serves the JSON API and the embedded React UI and runs the
collector, storing data in SQLite. No external database, broker or auth
service is needed.

## Quick start

```sh
git clone https://github.com/aurimasniekis/go-pwr-tracker.git
cd go-pwr-tracker
docker compose up -d --build
open http://localhost:8080
```

Or use a published image (`linux/amd64`, `linux/arm64`):

```sh
docker run -d --name pwr-tracker -p 8080:8080 -v pwr-data:/data \
  ghcr.io/aurimasniekis/pwr-tracker:latest
# also available as docker.io/aurimasniekis/pwr-tracker
```

Data is stored in the `pwr-data` volume and survives container recreation.
Standalone binaries (UI embedded) for Linux, macOS and Windows are attached to
each [GitHub Release](https://github.com/aurimasniekis/go-pwr-tracker/releases).

> **Responsible use.** The upstream is an undocumented endpoint of a
> commercial broadcaster. Keep the default 60 s interval, and respect the
> station's terms and any access restrictions. See
> [docs/operations.md](docs/operations.md#responsible-polling).

## Configuration

All settings are environment variables. Defaults are shown; see
[`.env.example`](.env.example). Durations use Go syntax (`30s`, `5m`,
`2160h`). Invalid values stop startup with a message listing every problem.

| Variable | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `DATABASE_PATH` | `/data/pwr-tracker.db` | SQLite file (local disk or Docker volume only) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. JSON logs on stdout, UTC timestamps |
| `UPSTREAM_URL` | `https://powerhitradio.tv3.lt/Pwr/lastSong/` | Endpoint to poll |
| `UPSTREAM_TIMEZONE` | `Europe/Vilnius` | Timezone of the upstream `date` field |
| `USER_AGENT` | `pwr-tracker/<version> (+repo URL)` | User-Agent for direct requests |
| `POLL_INTERVAL` | `60s` | Time between successful polls (minimum 1 s; keep 60 s in production) |
| `STARTUP_DELAY` | `2s` | Delay before the first poll |
| `HTTP_TIMEOUT` | `15s` | Timeout for direct upstream requests |
| `MAX_BACKOFF` | `15m` | Cap for exponential backoff after failures |
| `MAX_RESPONSE_BYTES` | `65536` | Largest accepted upstream body |
| `STALE_AFTER` | 3 × `POLL_INTERVAL` | Now-playing is marked stale after this long without success |
| `CONTINUITY_GAP` | 5 × `POLL_INTERVAL` | Without an upstream event id, observations further apart start a new, uncertain play |
| `COLLECTOR_ENABLED` | `true` | `false` serves existing data without polling |
| `FETCH_MODE` | `direct` | `direct` or `flaresolverr` (no automatic fallback) |
| `FLARESOLVERR_URL` | – | FlareSolverr base URL; required for `flaresolverr` mode |
| `FLARESOLVERR_TIMEOUT` | `60s` | FlareSolverr `maxTimeout` |
| `ANALYTICS_TIMEZONE` | `Europe/Vilnius` | IANA zone for day/hour buckets and default display |
| `OBSERVATION_RETENTION` | `2160h` (90 days) | Age after which observation rows are deleted; `0` keeps them. Plays are never deleted |
| `STORE_RAW_PAYLOADS` | `false` | Keep raw upstream bodies with observations |
| `RAW_PAYLOAD_RETENTION` | `168h` (7 days) | Age after which raw bodies are cleared |

### Optional FlareSolverr

Disabled by default and not needed today: direct requests work. If the
endpoint ever sits behind a challenge you are permitted to pass, enable it
explicitly:

```sh
printf 'FETCH_MODE=flaresolverr\nFLARESOLVERR_URL=http://flaresolverr:8191\n' >> .env
docker compose --profile flaresolverr up -d
```

FlareSolverr's port is not published; never expose it publicly. It cannot
solve every challenge. Solver failures are reported separately from upstream
HTTP errors.

## Data semantics

- **Observation**: one successful poll. **Play**: one airplay event. Repeated
  polls of the same item never increase play counts. All metrics count plays.
- **Identity**: the upstream provides a per-item `uid` (verified, see
  [docs/upstream.md](docs/upstream.md)), so consecutive plays of the same song
  are counted separately and restarts or retries never double-count. If a
  `uid` is ever missing, plays are inferred from track changes
  (A → B → A = 3 plays), and back-to-back repeats can't be detected.
- **Start times**: the upstream's `date` is used as an authoritative start when
  plausible. Otherwise the time the track was first observed is shown and
  labeled "first seen". Durations are not derived, since 60 s polling cannot
  measure them.
- **Uncertain plays**: in inference mode, the first play after startup or
  after a gap longer than `CONTINUITY_GAP` is marked uncertain. It is
  included in counts and reported separately.
- **Segments**: station items without an artist (e.g. "Eteryje -
  Informacija") are stored but excluded from analytics and hidden from the
  feed by default.
- **Normalization**: names are matched after Unicode NFC, whitespace
  collapsing and case folding only. Remix/edit/feat. information is kept, and
  collaborations like "A & B" are their own artist entries.
- **Time**: stored in UTC. Analytics use `ANALYTICS_TIMEZONE` with real
  DST rules (23/25-hour days). Ranges are half-open `[from, to)`. Partial
  buckets (in progress or before collection began) are flagged.
- **Comparisons** use the immediately preceding equal-length period. Zero
  previous plays yields `new`, never an infinite percentage. Comparisons are
  `unavailable` when collection had not started yet.
- **Gaps**: periods without successful polls are listed. Airplay during
  gaps is missing, not estimated.

## API

Read-only JSON under `/api/v1` (`now-playing`, `plays`, `tracks`, `artists`,
`stats/summary`, `stats/top-tracks`, `stats/top-artists`, `stats/airplay`,
`stats/hourly`, `status`) plus `/healthz` (liveness) and `/readyz` (database
readiness, independent of upstream health). See [docs/api.md](docs/api.md).

## Development

```sh
make dev-upstream   # terminal 1: deterministic fake upstream on :18081
make run            # terminal 2: API + collector on :8080 (tmp/dev.db)
cd web && npm ci && npm run dev   # terminal 3: UI with HMR on :5173, proxies /api
```

`make demo` serves 60 days of synthetic history from a separate `tmp/demo.db`
(the `demo-seed` command refuses to write into a non-empty database, so demo
data never mixes with real data). TanStack Router and Query devtools are
available in `npm run dev` only.

Checks: `make lint test e2e`. Details are in
[CONTRIBUTING.md](CONTRIBUTING.md).

Plain `go build ./...` produces an API-only binary. Build with
`-tags embedui` after `npm run build` to embed the UI (`make build`).

## Operations

Deployment, retention, health checks, and a WAL-safe **backup and restore**
procedure (`pwr-tracker backup`, which uses `VACUUM INTO`) are covered in
[docs/operations.md](docs/operations.md). Architecture, data model and the
reasoning behind SQLite are in [docs/architecture.md](docs/architecture.md).

## Limitations

- The upstream contract is inferred from observation and may change without
  notice (see [docs/upstream.md](docs/upstream.md) for uncertain fields).
- Items shorter than the poll interval (mostly station segments) can be
  missed entirely.
- Designed for one instance owning one SQLite file. No replicas, no network
  filesystems.
- No authentication; the API is read-only. Use a reverse proxy for TLS and
  rate limiting on public deployments.
- FlareSolverr mode is verified against FlareSolverr v3.5.2 on an
  unchallenged endpoint only.

## License

[MIT](LICENSE)
