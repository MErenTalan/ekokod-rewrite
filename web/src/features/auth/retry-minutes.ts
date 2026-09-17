/** Whole minutes from a 429's Retry-After seconds; at least one. */
export function retryMinutes(response: Response): number {
  const seconds = Number(response.headers.get('Retry-After'));
  return Number.isFinite(seconds) && seconds > 0 ? Math.ceil(seconds / 60) : 1;
}
