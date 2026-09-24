'use client';

import { api } from './client';

const FILENAME = /filename="?([^";]+)"?/i;

/**
 * Saves a server-rendered export (R198). It goes through the typed client's
 * fetch, so a 401 still triggers the single-flight refresh and retry (R168),
 * and the server's own Content-Disposition name wins over the fallback.
 */
export async function downloadFile(
  path: string,
  query: Record<string, string | undefined>,
  fallbackName: string,
): Promise<void> {
  const url = new URL(path, window.location.origin);
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== '') url.searchParams.set(key, value);
  }
  const response = await api.GET(url.pathname + url.search as never, { parseAs: 'blob' } as never);
  const blob = (response as { data?: Blob }).data;
  if (!blob) throw new Error('download failed');
  const name = FILENAME.exec(String((response as { response?: Response }).response?.headers.get('content-disposition') ?? ''))?.[1];
  const href = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement('a');
    anchor.href = href;
    anchor.download = name ?? fallbackName;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    URL.revokeObjectURL(href);
  }
}
