import type { Play } from '../api/types'
import { formatDate, formatTime } from '../lib/format'
import { reasonText } from '../lib/playText'
import { PlayMarkers } from './PlayMarkers'
import { TrackLabel } from './TrackLabel'

export function PlayList({
  plays,
  timeZone,
  showDate = false,
  hideTrack = false,
}: {
  plays: Play[]
  timeZone: string
  showDate?: boolean
  hideTrack?: boolean
}) {
  const dates = plays.map((p) => formatDate(p.start_at, timeZone))
  return (
    <ol className="play-list">
      {plays.map((p, i) => {
        const d = dates[i]!
        const header = showDate && d !== dates[i - 1]
        return (
          <li key={p.id} className={header ? 'with-date' : undefined}>
            {header && <div className="date-header">{d}</div>}
            <div className="play-row">
              <time
                dateTime={p.start_at}
                className="play-time"
                title={`${reasonText(p)}. Observed ${p.observation_count}× until ${formatTime(p.last_observed_at, timeZone)}.`}
              >
                {formatTime(p.start_at, timeZone)}
              </time>
              <span className="play-main">
                {hideTrack ? (
                  <span className="muted">Observed {p.observation_count}×</span>
                ) : (
                  <TrackLabel track={p.track} />
                )}
              </span>
              <span className="play-tags">
                <PlayMarkers play={p} />
              </span>
            </div>
          </li>
        )
      })}
    </ol>
  )
}
