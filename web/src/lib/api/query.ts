import { QueryClient } from '@tanstack/react-query';
import createQueryHooks from 'openapi-react-query';

import { api } from './client';
import { errorCodeOf } from './problem';

/** TanStack Query hooks over the typed client: `$api.useQuery('get', '/api/v1/...')`. */
export const $api = createQueryHooks(api);

/**
 * One QueryClient per browser session. A failed read throws the API's error
 * envelope, not an HTTP-aware error, so the envelope's code decides: anything
 * the API answered deliberately (a refusal, a rejection) is not retried, while
 * a server error or a transport failure is.
 */
export function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        retry: (failures, error) => {
          const code = errorCodeOf(error);
          return failures < 2 && (!code || code === 'internal');
        },
      },
    },
  });
}
