import type { ReactNode } from 'react'
import { ApiError } from '../api/client'

export function Loading({ label = 'Loading…' }: { label?: string }) {
  return (
    <div className="state state-loading" role="status" aria-live="polite">
      <span className="spinner" aria-hidden="true" />
      {label}
    </div>
  )
}

export function ErrorMessage({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const message =
    error instanceof ApiError
      ? error.message
      : error instanceof Error
        ? `Could not reach the server: ${error.message}`
        : 'Something went wrong.'
  return (
    <div className="state state-error" role="alert">
      <span>{message}</span>
      {onRetry && (
        <button type="button" className="btn btn-small" onClick={onRetry}>
          Retry
        </button>
      )}
    </div>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="state state-empty">{children}</div>
}
