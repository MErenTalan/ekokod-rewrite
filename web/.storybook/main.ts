import type { StorybookConfig } from '@storybook/nextjs-vite';

const config: StorybookConfig = {
  framework: '@storybook/nextjs-vite',
  stories: ['../src/components/**/*.stories.tsx', '../tests/a11y/fixtures/*.stories.tsx'],
  addons: ['@storybook/addon-a11y', '@storybook/addon-docs'],
  staticDirs: ['../public'],
  core: { disableTelemetry: true },
};

export default config;
