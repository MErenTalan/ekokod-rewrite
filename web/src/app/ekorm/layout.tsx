import { ModuleLayout } from '@/modules/module-layout';

// Every /ekorm page renders for one signed-in user; nothing here may be cached across users.
export const dynamic = 'force-dynamic';

export default function EkormLayout({ children }: { children: React.ReactNode }) {
  return ModuleLayout({ module: 'ekorm', children });
}
