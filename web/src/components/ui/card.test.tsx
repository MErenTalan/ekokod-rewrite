import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from './card';

describe('Card', () => {
  it('title level is configurable', () => {
    const { getByRole } = renderWithProviders(
      <Card>
        <CardHeader>
          <CardTitle as="h2">Son fatura</CardTitle>
          <CardDescription>Ağustos 2026</CardDescription>
        </CardHeader>
        <CardContent>₺12.345,60</CardContent>
        <CardFooter>Detay</CardFooter>
      </Card>,
    );
    expect(getByRole('heading', { level: 2, name: 'Son fatura' })).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <Card>
        <CardHeader>
          <CardTitle>Başlık</CardTitle>
        </CardHeader>
        <CardContent>İçerik</CardContent>
      </Card>,
    );
    await expectNoAxeViolations(container);
  });
});
