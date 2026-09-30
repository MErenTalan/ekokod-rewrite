import type { Metadata } from 'next';
import { redirect } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { authRedirectFor, CHOOSER_PATH, LOGIN_PATH } from '@/lib/api/errors';
import { getSession } from '@/lib/api/server';
import { modulesFor } from '@/modules/access';
import { ModuleChooser } from '@/modules/module-chooser';
import { MODULES } from '@/modules/registry';

export const dynamic = 'force-dynamic';

export async function generateMetadata(): Promise<Metadata> {
  const t = await getTranslations('shell.modules.chooser');
  return { title: t('title') };
}

/** R495: where a sign-in without `next` lands; a user with one module goes straight to it. */
export default async function ModulesPage() {
  const session = await getSession();
  if (!session.me) redirect(authRedirectFor(session.code ?? 'session_revoked', CHOOSER_PATH) ?? LOGIN_PATH);
  const available = modulesFor(session.me);
  if (available.length === 1) redirect(MODULES[available[0]].home);
  return <ModuleChooser name={session.me.name} modules={available} />;
}
