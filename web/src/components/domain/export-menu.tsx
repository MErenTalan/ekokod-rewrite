'use client';

import { Download, FileDown, FileSpreadsheet, FileText, Mail, type LucideIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useEffect, useState } from 'react';

import { Button } from '../ui/button';
import { DropdownMenu } from '../ui/dropdown-menu';
import { useAnnounce } from '../ui/live-announcer';

export type ExportFormat = 'csv' | 'excel' | 'pdf' | 'email';
export type ExportMenuProps = {
  formats?: ExportFormat[];
  onExport: (format: ExportFormat) => void | Promise<void>;
  busyFormat?: ExportFormat | null;
  disabled?: boolean;
};

const icons: Record<ExportFormat, LucideIcon> = { csv: FileText, excel: FileSpreadsheet, pdf: FileDown, email: Mail };

/** One export affordance everywhere (07 §6); generation itself happens server-side in F8. */
export function ExportMenu({ formats = ['csv', 'excel', 'pdf', 'email'], onExport, busyFormat = null, disabled = false }: ExportMenuProps) {
  const t = useTranslations('domain.export');
  const announce = useAnnounce();
  const [open, setOpen] = useState(false);
  useEffect(() => {
    if (busyFormat) announce(t('started', { format: t(busyFormat) }));
  }, [busyFormat, announce, t]);
  return (
    <DropdownMenu
      open={open}
      // Radix opens on pointerdown, which Button's loading guard (click) cannot stop (plan M-17).
      onOpenChange={(next) => setOpen(next && !busyFormat)}
      trigger={
        <Button variant="secondary" iconStart={Download} loading={Boolean(busyFormat)} disabled={disabled}>
          {t('menu')}
        </Button>
      }
      items={formats.map((format) => ({ type: 'item' as const, label: t(format), icon: icons[format], onSelect: () => void onExport(format) }))}
    />
  );
}
