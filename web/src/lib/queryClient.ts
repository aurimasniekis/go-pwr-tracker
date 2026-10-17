import { QueryClient } from '@tanstack/react-query'
import { ApiError } from '../api/client'

export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // Do not retry client errors; retry transient failures twice.
        retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
      },
    },
  })
}
