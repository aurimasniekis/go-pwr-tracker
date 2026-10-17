import { Link } from '@tanstack/react-router'
import type { ArtistCount, TrackCount } from '../api/types'
import { ChangeBadge } from './ChangeBadge'

function Meter({ value, max }: { value: number; max: number }) {
  return (
    <span className="meter" aria-hidden="true">
      <span style={{ width: `${max > 0 ? (value / max) * 100 : 0}%` }} />
    </span>
  )
}

export function TopTracksList({
  items,
  showArtist = true,
}: {
  items: TrackCount[]
  showArtist?: boolean
}) {
  const max = items[0]?.plays ?? 0
  return (
    <ol className="rank-list">
      {items.map((t, i) => (
        <li key={t.track.id}>
          <span className="rank">{i + 1}</span>
          <span className="rank-main">
            <Link to="/tracks/$trackId" params={{ trackId: t.track.id }} className="track-title">
              {t.track.title}
            </Link>
            {showArtist && t.track.artist && (
              <span className="muted"> · {t.track.artist.name}</span>
            )}
            <Meter value={t.plays} max={max} />
          </span>
          <span
            className="rank-count"
            title={t.uncertain_plays ? `${t.uncertain_plays} uncertain` : undefined}
          >
            {t.plays}
          </span>
          <ChangeBadge change={t.change} />
        </li>
      ))}
    </ol>
  )
}

export function TopArtistsList({ items }: { items: ArtistCount[] }) {
  const max = items[0]?.plays ?? 0
  return (
    <ol className="rank-list">
      {items.map((a, i) => (
        <li key={a.artist.id}>
          <span className="rank">{i + 1}</span>
          <span className="rank-main">
            <Link
              to="/artists/$artistId"
              params={{ artistId: a.artist.id }}
              className="track-title"
            >
              {a.artist.name}
            </Link>
            <span className="muted">
              {' '}
              · {a.distinct_tracks} track{a.distinct_tracks === 1 ? '' : 's'}
            </span>
            <Meter value={a.plays} max={max} />
          </span>
          <span className="rank-count">{a.plays}</span>
          <ChangeBadge change={a.change} />
        </li>
      ))}
    </ol>
  )
}
