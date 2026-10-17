import type { Summary } from '../api/types'
import { formatCount, formatDateTime, formatDuration } from '../lib/format'
import { ChangeBadge } from './ChangeBadge'

export function SummaryTiles({ data }: { data: Summary }) {
  const { totals, comparison } = data
  return (
    <div className="tiles">
      <div className="tile">
        <div className="tile-label">Plays</div>
        <div className="tile-value">{formatCount(totals.plays)}</div>
        <div className="tile-foot">
          <ChangeBadge change={comparison?.change} />
          {totals.uncertain_plays > 0 && (
            <span className="muted" title="Plays inferred after startup or a collection gap">
              {' '}
              incl. {totals.uncertain_plays} uncertain
            </span>
          )}
        </div>
      </div>
      <div className="tile">
        <div className="tile-label">Distinct tracks</div>
        <div className="tile-value">{formatCount(totals.distinct_tracks)}</div>
      </div>
      <div className="tile">
        <div className="tile-label">Distinct artists</div>
        <div className="tile-value">{formatCount(totals.distinct_artists)}</div>
      </div>
    </div>
  )
}

export function CoverageNote({ data, timeZone }: { data: Summary; timeZone: string }) {
  const { coverage } = data
  if (!coverage.partial) return null
  const notes: string[] = []
  if (
    !coverage.collection_started_at ||
    new Date(coverage.collection_started_at) > new Date(data.range.from)
  ) {
    notes.push(
      coverage.collection_started_at
        ? `Collection started ${formatDateTime(coverage.collection_started_at, timeZone)}, after the start of this period.`
        : 'No data has been collected yet.',
    )
  }
  if (coverage.gaps.length > 0) {
    notes.push(
      `${coverage.gaps.length}${coverage.gaps_truncated ? '+' : ''} collection gap${
        coverage.gaps.length === 1 ? '' : 's'
      } longer than ${formatDuration(coverage.min_gap_seconds)}; plays during gaps are missing.`,
    )
  }
  if (new Date(data.range.to) > new Date()) notes.push('The current period is still in progress.')
  if (notes.length === 0) return null
  return (
    <div className="coverage-note" role="note">
      <strong>Partial data.</strong> {notes.join(' ')}
      {coverage.gaps.length > 0 && (
        <details>
          <summary>Show gaps</summary>
          <ul>
            {coverage.gaps.map((g) => (
              <li key={g.from}>
                {formatDateTime(g.from, timeZone)} → {formatDateTime(g.to, timeZone)}
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}
