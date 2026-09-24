import { redirect } from 'next/navigation';

/** 01 §7.5 names /ekorm/predict; the screen lives at /ekorm/forecast (Q-I15). */
export default function Page(): never {
  redirect('/ekorm/forecast');
}
