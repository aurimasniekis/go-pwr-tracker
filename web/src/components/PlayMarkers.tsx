import type { Play } from '../api/types'
import { reasonText } from '../lib/playText'

/** Badges that explain how precise a play's start and identity are. */
export function PlayMarkers({ play }: { play: Play }) {
  return (
    <>
      {!play.start_authoritative && (
        <span
          className="tag tag-muted"
          title="The station did not report a start time; shown time is when the track was first observed"
        >
          first seen
        </span>
      )}
      {play.uncertain && (
        <span
          className="tag tag-warn"
          title={`${reasonText(play)}. May continue or overlap an earlier play.`}
        >
          uncertain
        </span>
      )}
    </>
  )
}
