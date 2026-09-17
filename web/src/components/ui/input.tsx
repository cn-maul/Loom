import * as React from 'react';
import { cn } from '../../lib/utils';

/** Apple text field: hairline edge, soft blue focus halo (not a hard ring). */
function Input({ className, ...props }: React.ComponentProps<'input'>) {
  return (
    <input
      data-slot="input"
      className={cn(
        'border-hairline bg-card h-11 w-full rounded-md border px-3.5 text-[14px] text-foreground shadow-xs outline-none transition-[border-color,box-shadow] placeholder:text-ink-4 focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/25',
        className,
      )}
      {...props}
    />
  );
}

export { Input };
