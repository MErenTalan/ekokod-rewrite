'use client';

import { useTranslations } from 'next-intl';

import type { HealthReport } from '@/lib/api';

export function HealthStatus({ report }: { report: HealthReport | null }) {
  const t = useTranslations('health');

  if (!report) {
    return (
      <p role="status" className="rounded-lg border border-danger bg-danger-subtle p-4 text-danger">
        {t('unreachable')}
      </p>
    );
  }

  return (
    <table className="w-full border-collapse text-start type-body">
      <thead>
        <tr className="border-b border-border">
          <th className="py-2">{t('check')}</th>
          <th className="py-2">{t('status')}</th>
          <th className="py-2">{t('duration')}</th>
        </tr>
      </thead>
      <tbody>
        {report.checks.map((check) => (
          <tr key={check.name} className="border-b border-border last:border-0">
            <td className="py-2">{check.name}</td>
            <td className="py-2">
              {check.status}
              {check.error ? <span className="ms-2 text-danger">{check.error}</span> : null}
            </td>
            <td className="py-2">{check.duration_ms} ms</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
