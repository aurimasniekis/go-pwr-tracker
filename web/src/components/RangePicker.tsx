import { PRESET_LABELS, RANGE_PRESETS, type RangePreset } from '../lib/range'

export function RangePicker({
  value,
  onChange,
}: {
  value: RangePreset
  onChange: (v: RangePreset) => void
}) {
  return (
    <div className="segmented" role="group" aria-label="Time range">
      {RANGE_PRESETS.map((p) => (
        <button
          key={p}
          type="button"
          aria-pressed={p === value}
          className={p === value ? 'active' : undefined}
          onClick={() => onChange(p)}
        >
          {PRESET_LABELS[p]}
        </button>
      ))}
    </div>
  )
}
