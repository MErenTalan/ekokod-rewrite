import { waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { mockApi } from '@/test/api-mock';
import { renderWithProviders } from '@/test/render';

import { LeadFormPanel } from './lead-form-panel';

let api: ReturnType<typeof mockApi>;
afterEach(() => api?.restore());

type Body = Record<string, unknown>;
function capture(path: string, response: () => Response) {
  const sent: Body[] = [];
  api = mockApi({ [`POST ${path}`]: (req: Request) => req.json().then((b: Body) => (sent.push(b), response())) });
  return sent;
}
const accepted = () => new Response(null, { status: 202 });

async function fillContact(r: ReturnType<typeof renderWithProviders>) {
  await r.user.type(r.getByLabelText(/Ad soyad/), 'Ayşe Yılmaz');
  await r.user.type(r.getByLabelText(/E-posta/), 'ayse@example.com');
  await r.user.type(r.getByLabelText(/Mesajınız/), 'Faturalarımızı kontrol etmek istiyoruz.');
}

describe('LeadFormPanel', () => {
  it('sends a contact message with the honeypot and elapsed time, then confirms', async () => {
    const sent = capture('/api/v1/public/contact', accepted);
    const r = renderWithProviders(<LeadFormPanel kind="contact" defaultSubject="Doküman talebi: Teknik Şartname" />);
    expect(r.getByLabelText(/Konu/)).toHaveValue('Doküman talebi: Teknik Şartname');
    await fillContact(r);
    await r.user.click(r.getByRole('button', { name: 'Gönder' }));
    expect(await r.findByText(/Mesajınız iletildi/)).toBeInTheDocument();
    expect(sent[0]).toMatchObject({ name: 'Ayşe Yılmaz', email: 'ayse@example.com', subject: 'Doküman talebi: Teknik Şartname', website: '' });
    expect(typeof sent[0].elapsed_ms).toBe('number');
    expect(sent[0].elapsed_ms as number).toBeGreaterThanOrEqual(0);
  });

  it('refuses an incomplete form without calling the API', async () => {
    api = mockApi({});
    const r = renderWithProviders(<LeadFormPanel kind="contact" />);
    await r.user.type(r.getByLabelText(/E-posta/), 'ayse@');
    await r.user.click(r.getByRole('button', { name: 'Gönder' }));
    expect(await r.findByRole('alert')).toHaveTextContent('Formu göndermeden önce şunları düzeltin');
    expect(r.getByLabelText(/E-posta/)).toHaveAccessibleDescription('Geçerli bir e-posta girin');
    expect(r.getByLabelText(/Mesajınız/)).toHaveAccessibleDescription('Zorunlu alan');
    expect(api.calls).toHaveLength(0);
  });

  it.each([
    [503, 'forms_not_configured', /İletişim formu şu anda kullanılamıyor.*info@ekokod\.com/],
    [502, 'delivery_failed', /Mesajınız iletilemedi.*info@ekokod\.com/],
    [429, 'rate_limited', /Çok fazla deneme yaptınız/],
  ])('explains a %i %s refusal and keeps what was typed', async (status, code, message) => {
    capture('/api/v1/public/contact', () => Response.json({ error: { code, message: code } }, { status }));
    const r = renderWithProviders(<LeadFormPanel kind="contact" />);
    await fillContact(r);
    await r.user.click(r.getByRole('button', { name: 'Gönder' }));
    expect(await r.findByText(message)).toBeInTheDocument();
    expect(r.getByLabelText(/Ad soyad/)).toHaveValue('Ayşe Yılmaz');
  });

  it('shows the API field errors of a 422', async () => {
    capture('/api/v1/public/contact', () => Response.json({ error: { code: 'validation_failed', message: 'x', details: { email: ['email'] } } }, { status: 422 }));
    const r = renderWithProviders(<LeadFormPanel kind="contact" />);
    await fillContact(r);
    await r.user.click(r.getByRole('button', { name: 'Gönder' }));
    await waitFor(() => expect(r.getByLabelText(/E-posta/)).toHaveAccessibleDescription('Geçerli bir e-posta girin'));
  });

  it('sends a demo request with company and phone to its own route', async () => {
    const sent = capture('/api/v1/public/demo-request', accepted);
    const r = renderWithProviders(<LeadFormPanel kind="demo" />);
    await r.user.type(r.getByLabelText(/Ad soyad/), 'Ayşe Yılmaz');
    await r.user.type(r.getByLabelText(/E-posta/), 'ayse@example.com');
    await r.user.type(r.getByLabelText(/Telefon/), '0555 111 22 33');
    await r.user.type(r.getByLabelText(/Şirket/), 'Acme Enerji');
    await r.user.click(r.getByRole('button', { name: 'Gönder' }));
    expect(await r.findByText(/Mesajınız iletildi/)).toBeInTheDocument();
    expect(sent[0]).toMatchObject({ company: 'Acme Enerji', phone: '0555 111 22 33', website: '' });
    expect(sent[0]).not.toHaveProperty('subject');
  });
});
