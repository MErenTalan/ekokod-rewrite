import { QueryClient } from '@tanstack/react-query';
import createQueryHooks from 'openapi-react-query';

import { api } from './client';

/** TanStack Query hooks over the typed client: `$api.useQuery('get', '/api/v1/...')`. */
export const $api = createQueryHooks(api);

/** One QueryClient per browser session; client errors (4xx) are not retried. */
export function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        retry: (failures, error) => {
          const status = (error as { status?: number } | null)?.status;
          return failures < 2 && !(status && status >= 400 && status < 500);
        },
      },
    },
  });
}
