/**
 * Enum value → message key. The API's enums are snake_case and the message
 * catalogues are camelCase (the i18n parity check enforces it), so the two
 * meet here rather than in every component.
 */
export function optionKey(value: string): string {
  return value.replace(/_(.)/g, (_, c: string) => c.toUpperCase());
}
