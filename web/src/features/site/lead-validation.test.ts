import { describe, expect, it } from 'vitest';

import { validateLead, type LeadValues } from './lead-validation';

const contact: LeadValues = { name: 'Ayşe', email: 'ayse@example.com', phone: '', subject: '', company: '', role: '', message: 'Bir sorum var, arar mısınız?' };

describe('validateLead (R353, client side so typos do not spend the rate limit)', () => {
  it('accepts a complete contact message', () => {
    expect(validateLead('contact', contact)).toEqual({});
  });

  it('needs a name, a valid e-mail and a message of 10–5000 characters', () => {
    expect(validateLead('contact', { ...contact, name: ' ', email: '', message: '' })).toEqual({ name: 'required', email: 'required', message: 'required' });
    expect(validateLead('contact', { ...contact, email: 'ayse@', message: 'kısa' })).toEqual({ email: 'email', message: 'tooShort' });
    expect(validateLead('contact', { ...contact, message: 'x'.repeat(5001) })).toEqual({ message: 'max' });
  });

  it('caps the optional fields the way the API does', () => {
    expect(validateLead('contact', { ...contact, phone: '1'.repeat(41), subject: 's'.repeat(201) })).toEqual({ phone: 'max', subject: 'max' });
  });

  it('a demo request needs phone and company; its message is optional', () => {
    const demo = { ...contact, message: '' };
    expect(validateLead('demo', demo)).toEqual({ phone: 'required', company: 'required' });
    expect(validateLead('demo', { ...demo, phone: '0555', company: 'Acme' })).toEqual({});
    expect(validateLead('demo', { ...demo, phone: '0555', company: 'Acme', message: 'x'.repeat(2001) })).toEqual({ message: 'max' });
  });
});
