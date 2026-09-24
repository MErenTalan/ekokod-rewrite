import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoEvaluation } from './_fixture';
import { EvaluateDialogView } from './evaluate-dialog';

const view = (evaluation = demoEvaluation) =>
  renderWithProviders(
    <EvaluateDialogView open alarmName="Endüktif izleme" evaluation={evaluation} onClose={() => {}} />,
  );

describe('EvaluateDialogView', () => {
  it('says plainly that nothing was sent or recorded (R223)', () => {
    const r = view();
    expect(r.getByText(/Bu bir denemedir/)).toBeVisible();
  });

  it('renders each breach as the sentence the server rendered', () => {
    const r = view();
    expect(r.getByText('Endüktif oran %25 eşiği aştı (%20)')).toBeVisible();
    expect(r.getByText('Tetiklendi')).toBeVisible();
  });

  it('renders a no-verdict as undecided, never as within limits (R216)', () => {
    const r = view();
    expect(r.getByText(/Karar verilemedi/)).toBeVisible();
    expect(r.getByText(/veri eksiktir/)).toBeVisible();
    // The undecided analyzer is NOT labelled as having fired.
    expect(r.getByText('Tetiklenmedi')).toBeVisible();
  });
});
