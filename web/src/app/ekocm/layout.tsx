import { ModuleLayout } from '@/modules/module-layout';

// Every /ekocm page renders for one signed-in user; nothing here may be cached across users.
export const dynamic = 'force-dynamic';

export default function EkocmLayout({ children }: { children: React.ReactNode }) {
  return ModuleLayout({ module: 'ekocm', children });
}
