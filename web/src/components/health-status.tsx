'use client';

import { useTranslations } from 'next-intl';

import type { HealthReport } from '@/lib/api';

export function HealthStatus({ report }: { report: HealthReport | null }) {
  const t = useTranslations('health');

  if (!report) {
    return (
      <p role="status" className="rounded-lg border border-red-300 bg-red-50 p-4 text-red-900">
        {t('unreachable')}
      </p>
    );
  }

  return (
    <table className="w-full border-collapse text-left text-sm">
      <thead>
        <tr className="border-b">
          <th className="py-2">{t('check')}</th>
          <th className="py-2">{t('status')}</th>
          <th className="py-2">{t('duration')}</th>
        </tr>
      </thead>
      <tbody>
        {report.checks.map((check) => (
          <tr key={check.name} className="border-b last:border-0">
            <td className="py-2">{check.name}</td>
            <td className="py-2">
              {check.status}
              {check.error ? <span className="ml-2 text-red-700">{check.error}</span> : null}
            </td>
            <td className="py-2">{check.duration_ms} ms</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
