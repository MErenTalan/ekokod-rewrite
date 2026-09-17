'use client';

import { ChevronDown } from 'lucide-react';
import { Accordion as RadixAccordion } from 'radix-ui';
import type { ReactNode } from 'react';

export type AccordionItem = { value: string; title: string; content: ReactNode };
export type AccordionProps = { items: AccordionItem[]; type: 'single' | 'multiple' };

export function Accordion({ items, type }: AccordionProps) {
  const children = items.map((item) => (
    <RadixAccordion.Item key={item.value} value={item.value} className="border-b border-border">
      <RadixAccordion.Header asChild>
        <h3>
          <RadixAccordion.Trigger className="group flex w-full items-center justify-between gap-2 py-3 text-start text-foreground type-body font-semibold pointer-coarse:min-h-11">
            {item.title}
            <ChevronDown aria-hidden className="size-4 shrink-0 text-foreground-muted transition-transform duration-(--duration-tab) group-data-[state=open]:rotate-180" />
          </RadixAccordion.Trigger>
        </h3>
      </RadixAccordion.Header>
      <RadixAccordion.Content className="pb-3 text-foreground-muted type-body">{item.content}</RadixAccordion.Content>
    </RadixAccordion.Item>
  ));
  return type === 'single' ? (
    <RadixAccordion.Root type="single" collapsible>
      {children}
    </RadixAccordion.Root>
  ) : (
    <RadixAccordion.Root type="multiple">{children}</RadixAccordion.Root>
  );
}
