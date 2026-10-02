type DotProps = { index?: number; cx?: number; cy?: number };

/**
 * A line draws segments, so a value with a gap on both sides draws nothing at
 * all: a month between two months without data vanished from the trend. Only
 * such values get a dot — every point dotted would clutter a dense series.
 */
export function isolatedDot(rows: Record<string, unknown>[], key: string, color: string) {
  const has = (i: number) => rows[i]?.[key] != null;
  return function IsolatedDot({ index = -1, cx, cy }: DotProps) {
    if (cx == null || cy == null || !has(index) || has(index - 1) || has(index + 1)) return <g key={index} />;
    return <circle key={index} data-isolated cx={cx} cy={cy} r={3.5} fill={color} />;
  };
}
