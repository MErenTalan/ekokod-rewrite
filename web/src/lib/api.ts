export type HealthCheck = {
  name: string;
  status: 'ok' | 'failed';
  error?: string;
  duration_ms: number;
};

export type HealthReport = {
  status: 'ok' | 'degraded';
  checks: HealthCheck[];
};

const apiBase =
  process.env.EKOKOD_INTERNAL_API_URL ?? process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080';

/** Reads the API's readiness report. Returns null when the API is unreachable. */
export async function fetchHealth(): Promise<HealthReport | null> {
  try {
    const response = await fetch(`${apiBase}/health/ready`, { cache: 'no-store' });
    return (await response.json()) as HealthReport;
  } catch {
    return null;
  }
}
