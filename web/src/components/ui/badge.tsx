import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';

/**
 * Tags/chips. Pill shape, small type, and grayscale by default — a tag is
 * metadata, not an accent. Callers that override the tone (a stale trait, a
 * failed extraction) pass the colour explicitly through `className`.
 */
const badgeVariants = cva(
  'inline-flex items-center justify-center gap-1 rounded-full border border-transparent px-2.5 py-[3px] text-[11.5px] font-medium w-fit whitespace-nowrap shrink-0 [&>svg]:size-3 [&>svg]:pointer-events-none',
  {
    variants: {
      variant: {
        default: 'bg-primary/10 text-primary',
        secondary: 'bg-fill text-ink-2',
        destructive: 'bg-destructive/10 text-destructive',
        outline: 'border-hairline text-ink-2',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

function Badge({ className, variant, ...props }: React.ComponentProps<'span'> & VariantProps<typeof badgeVariants>) {
  return <span data-slot="badge" className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { Badge, badgeVariants };
