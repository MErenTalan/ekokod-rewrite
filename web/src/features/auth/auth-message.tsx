import Link from 'next/link';

import { Alert, type AlertTone } from '@/components/ui/alert';
import { buttonVariants } from '@/components/ui/button';

/** A full auth page that is only a message and one way forward (auth error, maintenance). */
export function AuthMessage({ title, description, tone, action }: {
  title: string;
  description: string;
  tone: AlertTone;
  action: { href: string; label: string };
}) {
  return (
    <div className="flex flex-col gap-5">
      <h1 className="text-foreground type-h1">{title}</h1>
      <Alert tone={tone} title={description} />
      <Link href={action.href} className={buttonVariants({ variant: 'primary', size: 'lg' })}>
        {action.label}
      </Link>
    </div>
  );
}
