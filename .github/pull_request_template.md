## Summary

<!-- What does this change and why? Link related issues (Fixes #123). -->

## Checklist

- [ ] `go vet ./...` and `go test -race ./...` pass
- [ ] `npm run lint`, `npm run typecheck`, `npm test` pass in `web/` (if the UI changed)
- [ ] Docs / `.env.example` updated for new configuration or behavior
- [ ] New migrations are additive and numbered after the latest one
- [ ] No live upstream requests in tests
