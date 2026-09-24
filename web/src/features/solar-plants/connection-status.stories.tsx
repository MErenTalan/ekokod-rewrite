import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoRealtime } from './_fixture';
import { ConnectionStatus } from './connection-status';

const meta = {
  title: 'Features/SolarPlants/ConnectionStatus',
  component: ConnectionStatus,
  args: { realtime: demoRealtime },
} satisfies Meta<typeof ConnectionStatus>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Connected: Story = {};
export const AuthError: Story = { args: { realtime: { ...demoRealtime, connection: 'error', connection_error: 'isolar_auth' } } };
export const NeverSynced: Story = { args: { realtime: { ...demoRealtime, connection: 'never_synced', last_sync_at: undefined } } };
