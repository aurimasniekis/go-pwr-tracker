# Contributing

Thanks for helping! Small, focused pull requests are easiest to review.

## Development setup

Requirements: Go (version in `go.mod`), Node.js 24 LTS, npm.

```sh
# Terminal 1: deterministic fake upstream (no load on the real station)
go run ./cmd/fakeupstream -loop

# Terminal 2: API + collector against the fake upstream, local database
mkdir -p tmp
DATABASE_PATH=tmp/dev.db UPSTREAM_URL=http://127.0.0.1:18081/Pwr/lastSong/ \
  POLL_INTERVAL=5s LOG_LEVEL=debug go run ./cmd/pwr-tracker

# Terminal 3: Vite dev server with HMR, proxies /api to :8080
cd web && npm ci && npm run dev
```

For realistic history in charts, seed a separate demo database (it refuses to
write into a non-empty database):

```sh
go run ./cmd/pwr-tracker demo-seed -db tmp/demo.db -days 60
DATABASE_PATH=tmp/demo.db COLLECTOR_ENABLED=false go run ./cmd/pwr-tracker
```

`make help` lists shortcuts for the commands below.

## Checks

```sh
gofmt -l .                 # must print nothing
go vet ./...
go test -race ./...

cd web
npm run format:check
npm run lint
npm run typecheck
npm test
npm run build
npx playwright install chromium   # once
npm run e2e                       # needs the build above
```

Tests never contact the real upstream. Parser tests use captured fixtures in
`internal/upstream/testdata/`.

## Guidelines

- Keep the API read-only and versioned under `/api/v1`. Update
  `web/src/api/types.ts` and `docs/api.md` together with Go response types.
- Schema changes go in a new `internal/store/migrations/NNNN_name.sql`; never
  edit an applied migration.
- Prefer explicit SQL and the standard library over new dependencies.
- Do not add features that increase load on the upstream (poll interval
  minimum, retries) without discussion.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).
