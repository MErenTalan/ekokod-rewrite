'use client';

import { useTranslations } from 'next-intl';
import { useState } from 'react';

import { fieldErrors } from '@/lib/api/problem';
import { $api } from '@/lib/api/query';
import type { Translator } from '@/lib/api/types';

import { BillCalculator, type PublicBill } from './bill-calculator';
import { API_FIELDS, calculatorBody, initialValues, withChange, type CalcField } from './calculator-form';

/** POST /public/bill-calculator (F12a R351–R352); `today` is the server's Istanbul date. */
export function BillCalculatorPanel({ today }: { today: string }) {
  const forms = useTranslations('forms') as unknown as Translator;
  const [values, setValues] = useState(() => initialValues(today));
  const [errors, setErrors] = useState<Partial<Record<CalcField, string>>>({});
  const [result, setResult] = useState<PublicBill | null>(null);
  const calculate = $api.useMutation('post', '/api/v1/public/bill-calculator');

  const onSubmit = () => {
    const missing = Object.fromEntries((['start', 'end'] as const).filter((f) => !values[f]).map((f) => [f, forms('errors.required')]));
    setErrors(missing);
    if (Object.keys(missing).length) return;
    calculate.mutate(
      { body: calculatorBody(values) },
      {
        onSuccess: (bill) => setResult(bill ?? null),
        onError: (error) => {
          setResult(null);
          const byApi = fieldErrors(error, forms);
          setErrors(Object.fromEntries(Object.entries(byApi).map(([k, m]) => [API_FIELDS[k] ?? 'total', m])));
        },
      },
    );
  };

  return (
    <BillCalculator
      values={values}
      errors={errors}
      result={result}
      pending={calculate.isPending}
      onChange={(patch) => setValues((v) => withChange(v, patch))}
      onSubmit={onSubmit}
    />
  );
}
