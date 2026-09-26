import { CarbonSection } from '@/features/carbon/section-page';
import { sectionMetadata } from '@/modules/ekocm/metadata';

export const generateMetadata = () => sectionMetadata('reporting');

export default function Page() {
  return <CarbonSection section="reporting" />;
}
