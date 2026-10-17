# Upstream contract (observed)

The endpoint `https://powerhitradio.tv3.lt/Pwr/lastSong/` is **undocumented**.
Everything below was observed on 2026-10-17 by polling it every 30 seconds for
12 minutes (24 requests, all successful) and by reading the station website's
own JavaScript that consumes it. Treat it as an external dependency that can
change without notice.

## Transport

| Aspect | Observed |
|---|---|
| Method | `GET`, no parameters required (the site adds `?gold=1` for its "Gold" stream, not used here) |
| Status | `200` |
| `Content-Type` | `text/html; charset=UTF-8` (the body is JSON despite this header) |
| Encoding | UTF-8, no BOM, ~650–750 bytes |
| Caching | `cache-control: no-store, no-cache, must-revalidate`, `cf-cache-status: DYNAMIC` — responses are not cached at Cloudflare |
| CDN | Served through Cloudflare. Direct requests with a custom User-Agent succeeded; no challenge was presented during inspection |
| Cookies | Sets `PHPSESSID` and `__cf_bm`; not needed — pwr-tracker does not keep cookies |

## Body

```json
{
  "uid": "3086824",
  "artistName": "David Guetta & Anne-Marie & Coi Leray",
  "songTitle": "Baby Don't Hurt Me",
  "song": "David Guetta & Anne-Marie & Coi Leray - Baby Don't Hurt Me",
  "date": "2026-10-17 19:04:04",
  "dj": { "djname": "POWER MUZIKA", "program": "POWER MUZIKA", "timeon": "18:00:00", "...": "..." }
}
```

Fixtures: `internal/upstream/testdata/music.json`, `segment.json` (real
responses; they contained no personal data — `djemail` was empty).

| Field | Meaning (confidence) |
|---|---|
| `uid` | String of digits. Constant while an item is on air; increments by one for each new item, **including** short station segments, so it skipped values between our polls (e.g. 3086827 → 3086829). Used as the **play identity**. (High confidence.) |
| `artistName` | Artist display string. Collaborations appear in several forms (`A & B`, `A, B`, `A feat. B`) — never split. Empty for station segments. (High.) |
| `songTitle` | Title display string. Remix/version info is part of it. (High.) |
| `song` | `artistName - songTitle`, or just the title for segments. Redundant; used only if both structured fields are missing. |
| `date` | Local wall-clock time in **Europe/Vilnius** without an offset, format `YYYY-MM-DD HH:MM:SS`. Matches the item's start: e.g. a track first seen at 16:00:43Z had `date` 19:00:10 local (= 16:00:10Z), and the site's own JS uses `date` to detect a change of song. Treated as an **authoritative start** when it parses and is plausible (≤ 6 h before and ≤ 5 min after the observation). (Medium–high.) |
| `dj` | Current program/DJ metadata (name, photo path, schedule). Only `program`/`djname` are kept, as informational text. |

No artwork URL and no history list are provided.

### Station segments

Between songs the endpoint briefly reports items such as
`{"artistName": "", "songTitle": "Eteryje - Informacija"}` ("On air -
Information") lasting ~5–15 seconds. They get their own `uid`. pwr-tracker
stores them as tracks of kind `segment`, shows them in the activity feed only
on request, and **excludes them from all airplay analytics**.

## Uncertain points

- Whether `uid` is ever reused or reset (e.g. after a station system restart).
  If a known `uid` is later seen with a different track, pwr-tracker stops
  trusting it for that observation and falls back to transition inference.
- Whether `date` is the scheduled or actual start, and whether it can be
  updated for an already-running item. Only the first value seen is stored.
- Whether display strings may contain HTML entities. The station's site
  inserts `song` with jQuery `.html()`, so entities would render there; none
  were observed. pwr-tracker stores values as-is and always renders them as
  text.
- The DST behavior of `date` (no offset) during the autumn fall-back hour is
  inferred: the ambiguous candidate closest to the observation time is used.

## Through FlareSolverr

Verified against FlareSolverr v3.5.2: because the upstream labels its JSON as
`text/html`, the browser returns it as
`<html><head></head><body>{…json…}</body></html>` with HTML-escaped text.
pwr-tracker extracts the body text (or a `<pre>` block, should the content type
ever change) and then applies the same size limit, parser and validation as
the direct transport. Fixture: `testdata/flaresolverr_ok.json` (cookies
removed).
