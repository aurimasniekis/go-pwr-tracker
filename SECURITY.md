# Security policy

## Supported versions

Only the latest release receives security fixes.

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/aurimasniekis/go-pwr-tracker/security/advisories/new).
Do not open a public issue. You should receive a response within a week.

## Security model

- The HTTP API is **read-only** and unauthenticated. There are no
  administrative endpoints; maintenance (backup, demo seeding) is done with
  CLI subcommands inside the container or host.
- Production serves the UI and API from one origin. CORS is not enabled.
- Upstream values (artist, title, program) are untrusted and are only ever
  rendered as text, never as HTML.
- The container runs as a non-root user on a distroless image with no shell.
- **Never expose FlareSolverr to the internet.** It can be abused as an open
  browser proxy. The Compose profile does not publish its port.
- If you put pwr-tracker on the public internet, put it behind a reverse
  proxy with TLS and rate limiting; the API itself does no rate limiting.
