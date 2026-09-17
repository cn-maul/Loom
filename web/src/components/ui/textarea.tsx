import * as React from 'react';
import { cn } from '../../lib/utils';

/** Apple writing surface — the same field language as `Input`, taller. */
function Textarea({ className, ...props }: React.ComponentProps<'textarea'>) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        'border-hairline bg-card w-full rounded-md border px-3.5 py-3 text-[14px] text-foreground shadow-xs outline-none transition-[border-color,box-shadow] placeholder:text-ink-4 focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/25 disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    />
  );
}

export { Textarea };
