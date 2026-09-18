import type { JobRun, OperationalMessage } from '@/lib/api/types';

export const demoMessages = [
  {
    id: 1, kind: 'alarm', category: 'alarm-trigger', status: 'warning',
    message: 'Alarm tetiklendi', detail: 'Endüktif oran %25 eşiği aştı (%20)',
    related_type: 'alarm', related_id: 'al-1', created_at: '2026-09-18T11:00:00+03:00',
  },
  {
    id: 2, kind: 'job', category: 'analyzer-refresh', status: 'success',
    message: 'Analizör yenilendi', detail: null,
    related_type: null, related_id: null, created_at: '2026-09-18T10:00:00+03:00',
  },
  {
    id: 3, kind: 'system', category: 'auth', status: 'info',
    message: 'Oturum açıldı', detail: null,
    related_type: 'user', related_id: 'u-1', created_at: '2026-09-18T09:00:00+03:00',
  },
] as OperationalMessage[];

export const demoRuns = [
  {
    id: 'run-1', job_type: 'alarm.evaluate', scope: { company_id: 'c-own', rules: 4 },
    started_at: '2026-09-18T11:00:00+03:00', finished_at: '2026-09-18T11:00:12+03:00',
    status: 'partial', processed: 12, skipped: 3, failed: 1, error: null,
  },
  {
    id: 'run-2', job_type: 'billing.dispatch', scope: {},
    started_at: '2026-09-18T05:00:00+03:00', finished_at: null,
    status: 'running', processed: 0, skipped: 0, failed: 0, error: null,
  },
] as unknown as JobRun[];
