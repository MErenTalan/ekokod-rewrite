import { unitSymbol, type Unit } from '@/lib/format';

export type StatTileProps = { label: string; value: string; unit?: Unit; hint?: string };

/** `value` arrives preformatted (lib/format), so the tile never formats or rounds. */
export function StatTile({ label, value, unit, hint }: StatTileProps) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-lg border border-border bg-surface-raised p-4 in-data-[card=shadow]:border-card-edge in-data-[card=shadow]:shadow-md">
      <p className="text-foreground-muted type-caption">{label}</p>
      <p className="flex flex-wrap items-baseline gap-x-1.5 text-foreground">
        <span className="type-metric">{value}</span>
        {unit ? <span className="text-foreground-muted type-small">{unitSymbol(unit)}</span> : null}
      </p>
      {hint ? <p className="text-foreground-muted type-small">{hint}</p> : null}
    </div>
  );
}
