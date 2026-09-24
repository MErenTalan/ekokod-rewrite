export type LeadKind = 'contact' | 'demo';
export type LeadField = 'name' | 'email' | 'phone' | 'subject' | 'company' | 'role' | 'message';
export type LeadValues = Record<LeadField, string>;
export type LeadCode = 'required' | 'email' | 'max' | 'tooShort';

type Rule = { required?: boolean; max: number; min?: number };
const RULES: Record<LeadKind, Partial<Record<LeadField, Rule>>> = {
  contact: { name: { required: true, max: 120 }, email: { required: true, max: 254 }, phone: { max: 40 }, subject: { max: 200 }, message: { required: true, min: 10, max: 5000 } },
  demo: { name: { required: true, max: 120 }, email: { required: true, max: 254 }, phone: { required: true, max: 40 }, company: { required: true, max: 200 }, role: { max: 120 }, message: { max: 2000 } },
};
export const LEAD_FIELDS: Record<LeadKind, LeadField[]> = {
  contact: ['name', 'email', 'phone', 'subject', 'message'],
  demo: ['name', 'email', 'phone', 'company', 'role', 'message'],
};

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** R353's rules, checked before sending so a typo never spends the 5-per-10-minutes limit. */
export function validateLead(kind: LeadKind, values: LeadValues): Partial<Record<LeadField, LeadCode>> {
  const out: Partial<Record<LeadField, LeadCode>> = {};
  for (const [field, rule] of Object.entries(RULES[kind]) as [LeadField, Rule][]) {
    const v = values[field].trim();
    if (!v) {
      if (rule.required) out[field] = 'required';
      continue;
    }
    if ([...v].length > rule.max) out[field] = 'max';
    else if (field === 'email' && !EMAIL.test(v)) out[field] = 'email';
    else if (rule.min && [...v].length < rule.min) out[field] = 'tooShort';
  }
  return out;
}

export function isRequired(kind: LeadKind, field: LeadField): boolean {
  return RULES[kind][field]?.required ?? false;
}
