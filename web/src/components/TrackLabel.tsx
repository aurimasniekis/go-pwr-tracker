import { Link } from '@tanstack/react-router'
import type { TrackRef } from '../api/types'

/** Artist – title with links. Text only: upstream values are never rendered as HTML. */
export function TrackLabel({ track }: { track: TrackRef }) {
  if (track.kind === 'segment') {
    return (
      <span className="track-label">
        <span className="track-title">{track.title}</span>{' '}
        <span className="tag" title="Station segment (no artist): not counted as airplay">
          segment
        </span>
      </span>
    )
  }
  return (
    <span className="track-label">
      <Link to="/tracks/$trackId" params={{ trackId: track.id }} className="track-title">
        {track.title}
      </Link>
      {track.artist && (
        <>
          <span className="sep" aria-hidden="true">
            {' '}
            ·{' '}
          </span>
          <Link
            to="/artists/$artistId"
            params={{ artistId: track.artist.id }}
            className="track-artist"
          >
            {track.artist.name}
          </Link>
        </>
      )}
    </span>
  )
}
