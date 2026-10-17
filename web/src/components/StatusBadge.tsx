import type { CollectorStatus } from '../api/types'
import { formatRelative } from '../lib/format'

const LABELS: Record<CollectorStatus['state'], string> = {
  ok: 'Live',
  error: 'Update failed',
  stale: 'Stale',
  starting: 'Starting',
  disabled: 'Collector off',
}

const ICONS: Record<CollectorStatus['state'], string> = {
  ok: '●',
  error: '▲',
  stale: '■',
  starting: '○',
  disabled: '○',
}

export function StatusBadge({ status }: { status: CollectorStatus }) {
  const last = status.last_success_at
  const title = [
    last ? `Last successful update ${formatRelative(last)}` : 'No successful update yet',
    status.last_error ? `Last error: ${status.last_error}` : null,
  ]
    .filter(Boolean)
    .join('. ')
  return (
    <span className={`badge status status-${status.state}`} title={title}>
      <span aria-hidden="true">{ICONS[status.state]}</span> {LABELS[status.state]}
    </span>
  )
}
