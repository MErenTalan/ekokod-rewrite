// pnpm check:api — fails when src/lib/api/schema.d.ts is not what `pnpm gen:api` produces from the Go OpenAPI document.
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const root = join(import.meta.dirname, '..');
const dir = mkdtempSync(join(tmpdir(), 'ekokod-api-'));
try {
  const out = join(dir, 'schema.d.ts');
  execFileSync(
    join(root, 'node_modules/.bin/openapi-typescript'),
    ['../internal/api/v1/openapi.json', '-o', out],
    {
      cwd: root,
      stdio: 'ignore',
    },
  );
  if (readFileSync(out, 'utf8') !== readFileSync(join(root, 'src/lib/api/schema.d.ts'), 'utf8')) {
    console.error('src/lib/api/schema.d.ts is stale: run `make openapi && pnpm gen:api`');
    process.exit(1);
  }
  console.log('api schema ok');
} finally {
  rmSync(dir, { recursive: true, force: true });
}
