'use client';

import { useTranslations } from 'next-intl';

import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import type { CarbonCatalogue } from '@/lib/api/types';

import { isoKey, mainKey, scopeKey, subKey } from './labels';

export type ReferenceKind = 'ghg' | 'iso' | 'standards';

/** R327: the reference pages; the mapping table is the live catalogue, so it cannot drift from R301. */
export function ReferenceView({ kind, catalogue }: { kind: ReferenceKind; catalogue?: CarbonCatalogue }) {
  const t = useTranslations('carbon');
  if (kind === 'standards') {
    const rows = ['purpose', 'grouping', 'boundary', 'verification'] as const;
    return (
      <article className="flex flex-col gap-4">
        <h2 className="type-h2">{t('reference.standardsTitle')}</h2>
        <p className="max-w-prose text-foreground type-body">{t('reference.standardsIntro')}</p>
        <TableContainer label={t('reference.standardsLabel')}>
          <Table aria-label={t('reference.standardsLabel')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('reference.aspect')}</TableHead>
                <TableHead>{t('reference.ghgTitle')}</TableHead>
                <TableHead>{t('reference.isoTitle')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow key={row}>
                  <TableCell className="font-semibold">{t(`reference.rows.${row}`)}</TableCell>
                  <TableCell>{t(`reference.rows.${row}Ghg`)}</TableCell>
                  <TableCell>{t(`reference.rows.${row}Iso`)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </article>
    );
  }
  const points =
    kind === 'ghg'
      ? (['ghgScope1', 'ghgScope2', 'ghgScope3'] as const)
      : (['iso1', 'iso2', 'iso3', 'iso4', 'iso5', 'iso6'] as const);
  return (
    <article className="flex flex-col gap-4">
      <h2 className="type-h2">{t(kind === 'ghg' ? 'reference.ghgTitle' : 'reference.isoTitle')}</h2>
      <p className="max-w-prose text-foreground type-body">{t(kind === 'ghg' ? 'reference.ghgIntro' : 'reference.isoIntro')}</p>
      <ul className="flex max-w-prose list-disc flex-col gap-2 ps-5 text-foreground type-body">
        {points.map((p) => (
          <li key={p}>{t(`reference.${p}`)}</li>
        ))}
      </ul>
      {catalogue ? (
        <TableContainer label={t('reference.mappingLabel')}>
          <Table aria-label={t('reference.mappingLabel')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t(kind === 'ghg' ? 'reference.scope' : 'reference.iso')}</TableHead>
                <TableHead>{t('reference.activity')}</TableHead>
                <TableHead>{t('reference.category')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {/* Each page groups by its own standard: GHG by scope, ISO 14064-1 by category. */}
              {catalogue.items
                .flatMap((main) => main.subs.map((sub) => ({ main: main.key, sub })))
                .map((row, order) => ({ ...row, order, group: kind === 'ghg' ? row.sub.scope : row.sub.iso_category }))
                .sort((a, b) => a.group.localeCompare(b.group, 'en', { numeric: true }) || a.order - b.order)
                .map(({ main, sub }) => (
                  <TableRow key={sub.key}>
                    <TableCell className="whitespace-nowrap">{t(kind === 'ghg' ? scopeKey(sub.scope) : isoKey(sub.iso_category))}</TableCell>
                    <TableCell>{t(subKey(sub.key))}</TableCell>
                    <TableCell>{t(mainKey(main))}</TableCell>
                  </TableRow>
                ))}
            </TableBody>
          </Table>
        </TableContainer>
      ) : null}
    </article>
  );
}
