import { redirect } from 'next/navigation';

import { AppShell } from '@/components/shell/app-shell';
import { authRedirectFor } from '@/lib/api/errors';
import { getSession } from '@/lib/api/server';
import { SessionProvider } from '@/lib/session/session-provider';

// Every /ekorm page renders for one signed-in user; nothing here may be cached across users.
export const dynamic = 'force-dynamic';

export default async function EkormLayout({ children }: { children: React.ReactNode }) {
  const session = await getSession();
  // The middleware already refreshed a missing access cookie; a refusal here is a session the API ended.
  if (!session.me) redirect(session.code === 'device_mismatch' ? authRedirectFor(session.code, '/ekorm')! : '/auth/login');
  const { me } = session;
  return (
    <SessionProvider me={me}>
      <AppShell>{children}</AppShell>
    </SessionProvider>
  );
}
