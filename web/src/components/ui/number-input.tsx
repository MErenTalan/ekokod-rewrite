'use client';

import { useState } from 'react';

import { formatNumber, unitSymbol, type Unit } from '@/lib/format';
import { cn } from '@/lib/cn';

import { controlClasses, Field, type FieldProps } from './field';

export type NumberInputProps = FieldProps & {
  value: string | null;
  onValueChange: (value: string | null) => void;
  min?: string;
  max?: string;
  fractionDigits?: number;
  unit?: Unit;
  placeholder?: string;
};

const ALLOWED = /^-?[\d.,]*$/;

/** `1.234,56` → `'1234.56'` as a string, never through a float (plan D9, D11). Undefined when not a number. */
export function parseTurkishDecimal(text: string): string | null | undefined {
  const trimmed = text.replace(/\s/g, '');
  if (trimmed === '') return null;
  const normalised = trimmed.replace(/\./g, '').replace(',', '.');
  return /^-?\d+(\.\d+)?$/.test(normalised) ? normalised : undefined;
}

/** Sign-aware comparison of two plain decimal strings without floats. */
export function compareDecimal(a: string, b: string): number {
  const neg = (s: string) => s.startsWith('-');
  if (neg(a) !== neg(b)) return neg(a) ? -1 : 1;
  const sign = neg(a) ? -1 : 1;
  const [ai, af = ''] = a.replace('-', '').split('.');
  const [bi, bf = ''] = b.replace('-', '').split('.');
  const [ia, ib] = [ai.replace(/^0+(?=\d)/, ''), bi.replace(/^0+(?=\d)/, '')];
  if (ia.length !== ib.length) return (ia.length > ib.length ? 1 : -1) * sign;
  const width = Math.max(af.length, bf.length);
  const [x, y] = [ia + af.padEnd(width, '0'), ib + bf.padEnd(width, '0')];
  return x === y ? 0 : (x > y ? 1 : -1) * sign;
}

const display = (value: string | null, fractionDigits?: number) =>
  value === null
    ? ''
    : formatNumber(value, fractionDigits === undefined ? {} : { minFractionDigits: fractionDigits, maxFractionDigits: fractionDigits });

export function NumberInput({ label, description, error, required, id, disabled, value, onValueChange, min, max, fractionDigits, unit, placeholder }: NumberInputProps) {
  const [text, setText] = useState(() => display(value, fractionDigits));
  const [focused, setFocused] = useState(false);
  const [outOfRange, setOutOfRange] = useState(false);
  const shown = focused || outOfRange ? text : display(value, fractionDigits);
  return (
    <Field label={label} description={description} error={error} required={required} id={id} disabled={disabled}>
      {({ controlId, describedBy, invalid }) => (
        <div className="relative">
          <input
            id={controlId}
            type="text"
            inputMode="decimal"
            autoComplete="off"
            aria-describedby={describedBy}
            aria-invalid={invalid || outOfRange || undefined}
            required={required}
            disabled={disabled}
            placeholder={placeholder}
            value={shown}
            onFocus={() => {
              setText(display(value, fractionDigits));
              setFocused(true);
            }}
            onChange={(e) => {
              if (ALLOWED.test(e.target.value)) setText(e.target.value);
            }}
            onBlur={() => {
              setFocused(false);
              const parsed = parseTurkishDecimal(text);
              if (parsed === undefined) return;
              // Out-of-range input is kept for correction, never silently clamped.
              const beyond = parsed !== null && ((min !== undefined && compareDecimal(parsed, min) < 0) || (max !== undefined && compareDecimal(parsed, max) > 0));
              setOutOfRange(beyond);
              if (!beyond && parsed !== value) onValueChange(parsed);
            }}
            className={cn(controlClasses, 'text-end type-data', unit && 'pe-14')}
          />
          {unit ? (
            <span className="pointer-events-none absolute inset-y-0 end-3 flex items-center text-foreground-muted type-small">
              {unitSymbol(unit)}
            </span>
          ) : null}
        </div>
      )}
    </Field>
  );
}
