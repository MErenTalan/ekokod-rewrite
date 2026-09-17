// Catalogue checks for messages/{tr,en}/<namespace>.json (plan D7): key parity, ICU argument parity,
// no i18next {{…}} syntax, identifier keys.
type Catalogue = Record<string, unknown>;
const KEY = /^[a-zA-Z][a-zA-Z0-9]*$/;

export function flatten(obj: Catalogue, prefix = ''): Map<string, string> {
  const out = new Map<string, string>();
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === 'object') for (const [k, v] of flatten(value as Catalogue, path)) out.set(k, v);
    else out.set(path, String(value));
  }
  return out;
}

/** Top-level ICU argument names: `{name}` and `{name, plural …}`; nested plural/select branch bodies are skipped. */
export function icuArgs(message: string): string[] {
  const args = new Set<string>();
  let depth = 0;
  for (let i = 0; i < message.length; i++) {
    if (message[i] === '{') {
      const m = depth === 0 ? /^\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*[,}]/.exec(message.slice(i)) : null;
      if (m) args.add(m[1]);
      depth++;
    } else if (message[i] === '}') depth = Math.max(0, depth - 1);
  }
  return [...args].sort();
}

export function checkCatalogues(tr: Record<string, Catalogue>, en: Record<string, Catalogue>): string[] {
  const problems: string[] = [];
  const namespaces = [...new Set([...Object.keys(tr), ...Object.keys(en)])].sort();
  for (const ns of namespaces) {
    if (!tr[ns]) { problems.push(`tr/${ns}.json missing`); continue; }
    if (!en[ns]) { problems.push(`en/${ns}.json missing`); continue; }
    const flat = { tr: flatten(tr[ns]), en: flatten(en[ns]) };
    for (const locale of ['tr', 'en'] as const) {
      const other = locale === 'tr' ? 'en' : 'tr';
      for (const [key, value] of flat[locale]) {
        const full = `${ns}.${key}`;
        if (!flat[other].has(key)) problems.push(`${other}/${ns}.json missing ${full}`);
        if (value.includes('{{')) problems.push(`${locale}/${ns}.json: ${full} uses {{…}}`);
        if (locale === 'tr' && key.split('.').some((part) => !KEY.test(part))) {
          problems.push(`${ns}.json: invalid key "${full}"`);
        }
      }
    }
    for (const [key, value] of flat.tr) {
      const enValue = flat.en.get(key);
      if (enValue === undefined) continue;
      const a = icuArgs(value).join(',');
      const b = icuArgs(enValue).join(',');
      if (a !== b) problems.push(`${ns}.${key} ICU arguments differ: tr {${a}} vs en {${b}}`);
    }
  }
  return problems;
}
