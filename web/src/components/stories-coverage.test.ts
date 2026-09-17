// @vitest-environment node
import { existsSync, readdirSync } from 'node:fs';
import { basename, join } from 'node:path';

import { describe, expect, it } from 'vitest';

function components(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const path = join(dir, e.name);
    if (e.isDirectory()) return components(path);
    return e.name.endsWith('.tsx') && !/\.(test|stories)\.tsx$/.test(e.name) && e.name !== 'providers.tsx' ? [path] : [];
  });
}

describe('gallery coverage', () => {
  it('every component file has a story and a test', () => {
    const missing = components(import.meta.dirname).flatMap((file) =>
      // Internal `_` helpers need a test but no story.
      (basename(file).startsWith('_') ? ['.test.tsx'] : ['.stories.tsx', '.test.tsx'])
        .map((suffix) => file.replace(/\.tsx$/, suffix))
        .filter((sibling) => !existsSync(sibling))
        .map((sibling) => basename(sibling)),
    );
    expect(missing).toEqual([]);
  });

  it('every feature component has a story and a test', () => {
    const features = join(import.meta.dirname, '../features');
    const missing = components(features).flatMap((file) =>
      (basename(file).startsWith('_') ? ['.test.tsx'] : ['.stories.tsx', '.test.tsx']).map((suffix) => file.replace(/\.tsx$/, suffix)).filter((sibling) => !existsSync(sibling)).map((sibling) => basename(sibling)),
    );
    expect(missing).toEqual([]);
  });
});
