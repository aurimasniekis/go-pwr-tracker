import type { Change } from '../api/types'
import { describeChange } from '../lib/format'

export function ChangeBadge({ change }: { change: Change | null | undefined }) {
  if (!change) return null
  let text: string
  switch (change.state) {
    case 'up':
      text = `▲ ${change.percent}%`
      break
    case 'down':
      text = `▼ ${Math.abs(change.percent ?? 0)}%`
      break
    case 'flat':
      text = '= 0%'
      break
    case 'new':
      text = 'New'
      break
    case 'none':
      text = '–'
      break
    case 'unavailable':
      text = 'n/a'
      break
  }
  const desc = describeChange(change)
  return (
    <span className={`change change-${change.state}`} title={desc} aria-label={desc}>
      {text}
    </span>
  )
}
