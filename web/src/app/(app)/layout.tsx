import { AppShell } from '@/components/shell/app-shell';

// Placeholder user until F6 wires authentication.
const demoUser = { name: 'Demo Kullanıcı', email: 'demo@ekokod.com.tr', roleLabel: 'Yönetici' };

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return <AppShell user={demoUser}>{children}</AppShell>;
}
