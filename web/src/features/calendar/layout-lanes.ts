/** One event's position within a day, in minutes from midnight. */
export type TimedEvent = { id: string; startMin: number; endMin: number };

export type Lane = { lane: number; lanes: number };

/**
 * Side-by-side placement for events that overlap in a day column: each event
 * takes the first free lane, and every event of one overlapping cluster reports
 * the same lane count, so their widths add up to the column.
 */
export function layoutLanes(events: TimedEvent[]): Record<string, Lane> {
  const sorted = [...events].sort((a, b) => a.startMin - b.startMin || a.endMin - b.endMin);
  const out: Record<string, Lane> = {};
  let cluster: TimedEvent[] = [];
  let clusterEnd = -1;

  const flush = () => {
    if (cluster.length === 0) return;
    const lanes: number[] = []; // lane index → end minute
    const assigned = new Map<string, number>();
    for (const event of cluster) {
      let lane = lanes.findIndex((end) => end <= event.startMin);
      if (lane === -1) {
        lane = lanes.length;
        lanes.push(event.endMin);
      } else {
        lanes[lane] = event.endMin;
      }
      assigned.set(event.id, lane);
    }
    for (const event of cluster) out[event.id] = { lane: assigned.get(event.id) ?? 0, lanes: lanes.length };
    cluster = [];
    clusterEnd = -1;
  };

  for (const event of sorted) {
    if (cluster.length > 0 && event.startMin >= clusterEnd) flush();
    cluster.push(event);
    clusterEnd = Math.max(clusterEnd, event.endMin);
  }
  flush();
  return out;
}
