import { redirect } from 'next/navigation';

/** Self-registration stays disabled (01 §7.1, R151). */
export default function RegisterPage(): never {
  redirect('/auth/login');
}
