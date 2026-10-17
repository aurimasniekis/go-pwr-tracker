import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'
import { artistQuery, playsFeedQuery, trackQuery } from '../api/queries'
import { PlayList } from '../components/PlayList'
import { Empty, ErrorMessage, Loading } from '../components/QueryStates'
import { addDays } from '../lib/format'
import { useDisplayTimeZone } from '../lib/useTimeZone'

const route = getRouteApi('/activity')

export function Activity() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const tz = useDisplayTimeZone()
  const [q, setQ] = useState(search.q ?? '')

  // The URL keeps the inclusive "until" date users pick; the API range is
  // half-open, so the day after is sent as `to`.
  const filters = {
    q: search.q,
    from: search.from,
    to: search.to ? addDays(search.to, 1) : undefined,
    track_id: search.track_id,
    artist_id: search.artist_id,
    limit: 50,
  }
  const feed = useInfiniteQuery(playsFeedQuery(filters))
  const track = useQuery({ ...trackQuery(search.track_id ?? 0), enabled: !!search.track_id })
  const artist = useQuery({ ...artistQuery(search.artist_id ?? 0), enabled: !!search.artist_id })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    navigate({ search: (s) => ({ ...s, q: q.trim() || undefined }) })
  }
  const plays = feed.data?.pages.flatMap((p) => p.items) ?? []
  const hasFilters = !!(search.q || search.from || search.to || search.track_id || search.artist_id)

  return (
    <section className="card">
      <div className="card-head">
        <h1>Activity</h1>
        <span className="muted">Newest first · times in {tz}</span>
      </div>

      <form className="filters" onSubmit={onSubmit} role="search">
        <label>
          <span>Track or artist</span>
          <input
            type="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search…"
            maxLength={100}
          />
        </label>
        <label>
          <span>From</span>
          <input
            type="date"
            value={search.from ?? ''}
            onChange={(e) =>
              navigate({ search: (s) => ({ ...s, from: e.target.value || undefined }) })
            }
          />
        </label>
        <label>
          <span>Until (inclusive)</span>
          <input
            type="date"
            value={search.to ?? ''}
            onChange={(e) =>
              navigate({ search: (s) => ({ ...s, to: e.target.value || undefined }) })
            }
          />
        </label>
        <button type="submit" className="btn">
          Apply
        </button>
        {hasFilters && (
          <button
            type="button"
            className="btn btn-ghost"
            onClick={() => {
              setQ('')
              navigate({ search: {} })
            }}
          >
            Clear
          </button>
        )}
      </form>

      {(search.track_id || search.artist_id) && (
        <p className="active-filters">
          Filtered to{' '}
          {search.track_id && <strong>track “{track.data?.title ?? `#${search.track_id}`}”</strong>}
          {search.artist_id && (
            <strong>artist “{artist.data?.name ?? `#${search.artist_id}`}”</strong>
          )}{' '}
          <button
            type="button"
            className="link-button"
            onClick={() =>
              navigate({ search: (s) => ({ ...s, track_id: undefined, artist_id: undefined }) })
            }
          >
            remove
          </button>
        </p>
      )}

      {feed.isPending ? (
        <Loading />
      ) : feed.isError ? (
        <ErrorMessage error={feed.error} onRetry={() => feed.refetch()} />
      ) : plays.length === 0 ? (
        <Empty>{hasFilters ? 'No plays match these filters.' : 'No plays recorded yet.'}</Empty>
      ) : (
        <>
          <PlayList plays={plays} timeZone={tz} showDate />
          <div className="load-more">
            {feed.hasNextPage ? (
              <button
                type="button"
                className="btn"
                onClick={() => feed.fetchNextPage()}
                disabled={feed.isFetchingNextPage}
              >
                {feed.isFetchingNextPage ? 'Loading…' : 'Load more'}
              </button>
            ) : (
              <span className="muted">End of history</span>
            )}
          </div>
        </>
      )}
      <p className="muted small">
        “first seen” marks plays whose start time was not reported by the station; the time shown is
        when the collector first observed the track. “uncertain” marks plays inferred after startup
        or a collection gap.
      </p>
    </section>
  )
}
