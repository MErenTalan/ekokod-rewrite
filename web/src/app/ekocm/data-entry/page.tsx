import { CarbonSection } from '@/features/carbon/carbon-section';
import { sectionMetadata } from '@/modules/ekocm/metadata';

export const generateMetadata = () => sectionMetadata('entry');

export default function Page() {
  return <CarbonSection section="entry" />;
}
