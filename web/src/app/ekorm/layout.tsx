import { redirect } from 'next/navigation';

import { AppShell } from '@/components/shell/app-shell';
import { getMe } from '@/lib/api/server';
import { SessionProvider } from '@/lib/session/session-provider';

// Every /ekorm page renders for one signed-in user; nothing here may be cached across users.
export const dynamic = 'force-dynamic';

export default async function EkormLayout({ children }: { children: React.ReactNode }) {
  const me = await getMe();
  // The middleware already refreshed a missing access cookie; a null here is a session the API refuses.
  if (!me) redirect('/auth/login');
  return (
    <SessionProvider me={me}>
      <AppShell>{children}</AppShell>
    </SessionProvider>
  );
}
