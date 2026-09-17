import { redirect } from 'next/navigation';

/** Until the public site (F12) the root is the platform (R167); the middleware redirects first. */
export default function RootPage(): never {
  redirect('/ekorm');
}
