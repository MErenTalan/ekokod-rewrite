// @vitest-environment node
import { ESLint } from 'eslint';
import { beforeAll, describe, expect, it } from 'vitest';

let eslint: ESLint;
beforeAll(() => {
  eslint = new ESLint({ cwd: `${import.meta.dirname}/../..` });
});

async function ruleIds(code: string) {
  const [result] = await eslint.lintText(code, { filePath: 'src/components/ui/x.tsx' });
  return result.messages.map((m) => `${m.ruleId}: ${m.message}`);
}

const mustError: [string, string, string][] = [
  ['arbitrary hex', 'export const A = () => <div className="bg-[#fff]" />;', 'Raw colour literal'],
  ['rgb in a template', 'export const c = `text-[rgb(0_0_0)]`;', 'Raw colour literal'],
  ['default palette', 'export const A = () => <div className="bg-red-500" />;', 'Tailwind default palette'],
  ['palette with opacity', 'export const A = () => <div className="bg-red-500/50" />;', 'Tailwind default palette'],
  ['white and black', 'export const A = () => <div className="text-white border-black" />;', 'Tailwind default palette'],
  ['palette behind a variant', 'export const A = () => <div className="hover:text-neutral-900" />;', 'Tailwind default palette'],
  ['energy colour as text', 'export const A = () => <div className="text-consumption" />;', 'graphics only'],
  ['outline-none', 'export const A = () => <button className="outline-none" />;', 'outline-none / dark:'],
  ['dark variant', 'export const A = () => <div className="dark:bg-surface" />;', 'outline-none / dark:'],
  ['recharts outside charts', "import { LineChart } from 'recharts';\nexport const x = LineChart;", 'src/components/charts'],
];

const mustPass: [string, string][] = [
  ['skip-link anchor', 'export const A = () => <a href="#main">x</a>;'],
  ['hex-looking anchor', 'export const A = () => <a href="#add-row">x</a>;'],
  ['token utilities', 'export const A = () => <div className="bg-primary text-on-primary" />;'],
  ['token with opacity', 'export const A = () => <div className="bg-primary/50" />;'],
  ['energy colour as fill', 'export const A = () => <svg className="fill-consumption" />;'],
];

describe('raw colour lint', () => {
  for (const [name, code, message] of mustError) {
    it(`rejects ${name}`, { timeout: 30_000 }, async () => {
      expect((await ruleIds(code)).some((m) => m.includes(message))).toBe(true);
    });
  }
  for (const [name, code] of mustPass) {
    it(`allows ${name}`, { timeout: 30_000 }, async () => {
      expect(await ruleIds(code)).toEqual([]);
    });
  }
});
