import type { NowPlaying } from '../api/types'
import { formatRelative, formatTime } from '../lib/format'
import { PlayMarkers } from './PlayMarkers'
import { StatusBadge } from './StatusBadge'
import { TrackLabel } from './TrackLabel'

export function NowPlayingCard({ data, timeZone }: { data: NowPlaying; timeZone: string }) {
  const { play, collector } = data
  return (
    <section className="card now-playing" aria-labelledby="now-playing-h">
      <div className="card-head">
        <h2 id="now-playing-h">{collector.stale && play ? 'Last seen on air' : 'Now playing'}</h2>
        <StatusBadge status={collector} />
      </div>
      {play ? (
        <div className="np-body">
          <div className="np-track">
            <TrackLabel track={play.track} />
          </div>
          <div className="np-meta">
            {play.start_authoritative ? 'Started' : 'First seen'} at{' '}
            <time dateTime={play.start_at}>{formatTime(play.start_at, timeZone)}</time>
            {' · '}last seen{' '}
            <time dateTime={play.last_observed_at}>
              {formatTime(play.last_observed_at, timeZone)}
            </time>{' '}
            <PlayMarkers play={play} />
          </div>
        </div>
      ) : (
        <p className="muted">Nothing observed yet. The first poll runs shortly after startup.</p>
      )}
      <p className="np-status muted">
        {collector.last_success_at ? (
          <>
            Last successful update{' '}
            <time dateTime={collector.last_success_at}>
              {formatRelative(collector.last_success_at)}
            </time>
          </>
        ) : (
          'No successful update yet'
        )}
        {collector.stale && collector.state !== 'starting' && (
          <strong className="warn-text"> · Data may be out of date</strong>
        )}
        {collector.consecutive_failures > 0 && collector.last_error && (
          <span className="error-text"> · {collector.last_error}</span>
        )}
      </p>
    </section>
  )
}
