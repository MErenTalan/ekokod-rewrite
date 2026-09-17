import { vi } from 'vitest';

// Backs the next/navigation mock in vitest.setup.ts (plan I-8).
export const mockPathname = { current: '/' };
export const mockRouter = {
  push: vi.fn(),
  replace: vi.fn(),
  refresh: vi.fn(),
  back: vi.fn(),
  prefetch: vi.fn(),
};

export function setMockPathname(path: string): void {
  mockPathname.current = path;
}
