import { afterEach, describe, expect, it, vi } from 'vitest';

import { mockApi, queryOf } from '@/test/api-mock';

import { downloadFile } from './download';

let api: ReturnType<typeof mockApi>;
afterEach(() => {
  api?.restore();
  vi.restoreAllMocks();
});

describe('downloadFile', () => {
  it('saves the file under the name the server chose and forwards the scope', async () => {
    api = mockApi({
      'GET /api/v1/consumption/export': () =>
        new Response('a;b\r\n1;2', {
          headers: { 'content-type': 'text/csv', 'content-disposition': 'attachment; filename="tuketim-20260801.csv"' },
        }),
    });
    const created = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:x');
    const revoked = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const clicks: HTMLAnchorElement[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      clicks.push(this);
    });

    await downloadFile('/api/v1/consumption/export', { format: 'csv', analyzer_id: 'a-1', company_id: 'c-b', empty: undefined }, 'yedek.csv');

    expect(clicks[0].download).toBe('tuketim-20260801.csv');
    expect(created).toHaveBeenCalled();
    expect(revoked).toHaveBeenCalledWith('blob:x');
    const query = queryOf(api.calls, 'GET', '/api/v1/consumption/export');
    expect(query.get('company_id')).toBe('c-b');
    expect(query.has('empty')).toBe(false);
  });

  it('falls back to the caller name when the server sends none', async () => {
    api = mockApi({ 'GET /api/v1/load-profile/export': () => new Response('x') });
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:y');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const clicks: HTMLAnchorElement[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      clicks.push(this);
    });
    await downloadFile('/api/v1/load-profile/export', {}, 'yuk-profili.xlsx');
    expect(clicks[0].download).toBe('yuk-profili.xlsx');
  });
});
