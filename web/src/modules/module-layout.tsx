import { headers } from 'next/headers';
import { redirect } from 'next/navigation';

import { AppShell } from '@/components/shell/app-shell';
import { authRedirectFor, CHOOSER_PATH } from '@/lib/api/errors';
import { getSession } from '@/lib/api/server';
import { SessionProvider } from '@/lib/session/session-provider';
import { PATH_HEADER } from '@/middleware';

import { modulesFor } from './access';
import { MODULES, type ModuleId } from './registry';

/** One module's server layout: the shared session guard, the entitlement gate (R491) and the module's shell. */
export async function ModuleLayout({ module, children }: { module: ModuleId; children: React.ReactNode }) {
  const session = await getSession();
  // The middleware already refreshed a missing access cookie; a refusal here is a session the API ended.
  if (!session.me) {
    const path = (await headers()).get(PATH_HEADER) ?? MODULES[module].home;
    redirect(authRedirectFor(session.code ?? 'session_revoked', path) ?? '/auth/login');
  }
  const { me } = session;
  if (!modulesFor(me).includes(module)) redirect(CHOOSER_PATH);
  return (
    <SessionProvider me={me}>
      <AppShell module={module}>{children}</AppShell>
    </SessionProvider>
  );
}
