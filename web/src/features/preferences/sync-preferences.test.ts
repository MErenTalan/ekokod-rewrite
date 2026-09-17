import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { createPreferenceSync, type PreferenceBody } from './sync-preferences';

const body = (theme: string, locale: 'tr' | 'en' = 'tr'): PreferenceBody => ({ ui_preferences: `{"theme":"${theme}"}`, locale });

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('createPreferenceSync', () => {
  it('sends one PATCH 500 ms after the last change', async () => {
    const send = vi.fn(async () => {});
    const sync = createPreferenceSync(send, body('light'));
    sync.push(body('dark'));
    await vi.advanceTimersByTimeAsync(300);
    sync.push(body('system'));
    await vi.advanceTimersByTimeAsync(499);
    expect(send).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(send).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenCalledWith(body('system'));
  });

  it('does not send what the profile already holds', async () => {
    const send = vi.fn(async () => {});
    const sync = createPreferenceSync(send, body('light', 'en'));
    sync.push(body('light', 'en'));
    await vi.advanceTimersByTimeAsync(1000);
    expect(send).not.toHaveBeenCalled();
    sync.push(body('light', 'tr'));
    await vi.advanceTimersByTimeAsync(500);
    expect(send).toHaveBeenCalledWith(body('light', 'tr'));
  });

  it('retries a failed save on the next push even if the value is unchanged', async () => {
    const send = vi.fn(async () => {
      throw new Error('503');
    });
    const sync = createPreferenceSync(send, body('light'));
    sync.push(body('dark'));
    await vi.advanceTimersByTimeAsync(500);
    sync.push(body('dark'));
    await vi.advanceTimersByTimeAsync(500);
    expect(send).toHaveBeenCalledTimes(2);
  });

  it('cancel drops a pending save', async () => {
    const send = vi.fn(async () => {});
    const sync = createPreferenceSync(send, body('light'));
    sync.push(body('dark'));
    sync.cancel();
    await vi.advanceTimersByTimeAsync(1000);
    expect(send).not.toHaveBeenCalled();
  });
});
