// Dependency-free so the edge middleware can import it without the module registry's icons.
export const MODULE_IDS = ['ekorm', 'ekocm'] as const;
export type ModuleId = (typeof MODULE_IDS)[number];
export const MODULE_PREFIXES = MODULE_IDS.map((id) => `/${id}` as const);
