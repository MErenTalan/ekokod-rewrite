import { vi } from 'vitest';

export type MockResponder = (request: Request) => Response | Promise<Response>;
/** A route's answer: a JSON body, a Response, or a function of the request. */
export type MockRoute = unknown | Response | MockResponder;

const NOT_FOUND = () =>
  Response.json({ error: { code: 'not_found', message: 'Bulunamadı' } }, { status: 404 });

/**
 * Stubs `fetch` with a route table keyed `"<METHOD> <pathname>"`, records every
 * request, and answers anything unmapped with the API's own 404 envelope — so a
 * container test fails loudly on a call it did not expect.
 */
export function mockApi(routes: Record<string, MockRoute>): { calls: Request[]; restore: () => void } {
  const calls: Request[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : new Request(input, init);
    calls.push(request);
    const { pathname } = new URL(request.url);
    const route = routes[`${request.method} ${pathname}`];
    if (route === undefined) return NOT_FOUND();
    if (typeof route === 'function') return (route as MockResponder)(request);
    if (route instanceof Response) return route.clone();
    return Response.json(route);
  });
  vi.stubGlobal('fetch', fetchMock);
  return { calls, restore: () => vi.unstubAllGlobals() };
}

/** The query string of the nth recorded call to a path, for assertions. */
export function queryOf(calls: Request[], method: string, pathname: string, index = 0): URLSearchParams {
  const matches = calls.filter((c) => c.method === method && new URL(c.url).pathname === pathname);
  return new URL(matches[index]?.url ?? 'http://x/').searchParams;
}
