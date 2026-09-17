import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';

/**
 * Apple buttons: pill radius, generous hit area (44px default), feedback on
 * pointer-DOWN (`active:scale`) rather than on release.
 *
 * `sm` is the deliberate exception at 36px — it exists for dense table rows
 * and toolbars, where the surrounding row already provides the target.
 */
const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-full text-[14px] font-semibold tracking-[-0.005em] transition-[background-color,color,box-shadow,filter,transform] duration-200 active:scale-[0.97] disabled:pointer-events-none disabled:opacity-40 [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 shrink-0 [&_svg]:shrink-0 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/30",
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground hover:brightness-[1.08]',
        destructive: 'bg-destructive text-white hover:brightness-[1.08]',
        // The skill's "secondary / light glass" button: a near-white plate
        // with a hairline edge, not a tinted one.
        outline: 'border border-hairline bg-card/80 text-foreground shadow-xs hover:bg-card',
        secondary: 'bg-fill text-foreground hover:brightness-[0.96]',
        ghost: 'text-ink-2 hover:bg-fill hover:text-foreground',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-11 px-5',
        sm: 'h-9 px-4 text-[13px]',
        lg: 'h-12 px-7 text-[15px]',
        icon: 'size-11',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

function Button({
  className,
  variant,
  size,
  loading = false,
  children,
  ...props
}: React.ComponentProps<'button'> & VariantProps<typeof buttonVariants> & { loading?: boolean }) {
  return (
    <button
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      disabled={loading}
      {...props}
    >
      {loading && (
        <span className="size-4 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent" />
      )}
      {children}
    </button>
  );
}

export { Button, buttonVariants };
