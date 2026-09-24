import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { demoBuildingStates, demoIcmalImport } from './_fixture';
import { IcmalTabView } from './icmal-tab';

const meta = {
  title: 'Features/Tariffs/IcmalTab',
  component: IcmalTabView,
  args: { result: demoIcmalImport, buildings: demoBuildingStates, onUpload: () => {}, onApply: () => {} },
} satisfies Meta<typeof IcmalTabView>;
export default meta;
type Story = StoryObj<typeof meta>;

/** Step one: nothing has been uploaded yet. */
export const Upload: Story = { args: { result: null } };
export const Review: Story = {};
/** 02 §8.4's 2 % target missed: the operator is told before confirming. */
export const OutOfTolerance: Story = {
  args: {
    result: { ...demoIcmalImport, analyses: [{ ...demoIcmalImport.analyses[0], within_tolerance: false }] },
  },
};
export const Unreadable: Story = { args: { result: null, failure: 'unreadable' } };
