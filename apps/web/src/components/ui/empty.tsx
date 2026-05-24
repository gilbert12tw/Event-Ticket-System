import type { ComponentProps } from "react";

import { cn } from "@/lib/utils";

function Empty({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="empty"
      className={cn(
        "flex w-full min-w-0 flex-1 flex-col items-center justify-center gap-4 rounded-xl border-dashed p-6 text-center text-balance",
        className,
      )}
      {...props}
    />
  );
}

function EmptyHeader({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="empty-header"
      className={cn("flex max-w-sm flex-col items-center gap-2", className)}
      {...props}
    />
  );
}

function EmptyTitle({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="empty-title"
      className={cn(
        "font-heading text-sm font-medium tracking-tight",
        className,
      )}
      {...props}
    />
  );
}

function EmptyDescription({ className, ...props }: ComponentProps<"p">) {
  return (
    <div
      data-slot="empty-description"
      className={cn(
        "text-sm/relaxed text-muted-foreground [&>a]:underline [&>a]:underline-offset-4 [&>a:hover]:text-primary",
        className,
      )}
      {...props}
    />
  );
}

export { Empty, EmptyDescription, EmptyHeader, EmptyTitle };
