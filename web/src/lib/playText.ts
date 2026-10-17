import type { Play } from '../api/types'

const REASON_TEXT: Record<Play['reason'], string> = {
  upstream_event: 'New upstream event',
  observed_transition: 'Inferred from a change in the observed track',
  initial_observation: 'First observation after the collector started',
  continuity_gap: 'First observation after a collection gap',
}

/** Why a play segment was created, in plain language. */
export function reasonText(play: Play): string {
  return REASON_TEXT[play.reason]
}
