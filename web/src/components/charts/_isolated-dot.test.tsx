import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { isolatedDot } from './_isolated-dot';

const rows = [{ a: 1 }, { a: null }, { a: 3 }, { a: 4 }, { a: null }];
const dots = (index: number) => {
  const Dot = isolatedDot(rows, 'a', 'red');
  return render(<svg><Dot index={index} cx={10} cy={20} /></svg>).container.querySelectorAll('circle[data-isolated]');
};

describe('isolatedDot', () => {
  it('marks a value with a gap on both sides', () => {
    expect(dots(0)).toHaveLength(1);
  });

  it('leaves a value that a segment already reaches', () => {
    expect(dots(2)).toHaveLength(0);
    expect(dots(3)).toHaveLength(0);
  });

  it('draws nothing where there is no value', () => {
    expect(dots(1)).toHaveLength(0);
  });
});
