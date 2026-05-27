import type { ComponentProps } from "react";

import { cn } from "@/lib/utils";

const alertBase =
  "group/alert relative grid w-full gap-0.5 rounded-[var(--radius)] border px-2.5 py-2 text-left text-sm has-data-[slot=alert-action]:relative has-data-[slot=alert-action]:pr-18 has-[>svg]:grid-cols-[auto_1fr] has-[>svg]:gap-x-2 *:[svg]:row-span-2 *:[svg]:translate-y-0.5 *:[svg]:text-current *:[svg:not([class*='size-'])]:size-4";

const alertVariantClasses = {
  default: "bg-card text-card-foreground",
  destructive:
    "bg-card text-destructive *:data-[slot=alert-description]:text-destructive/90 *:[svg]:text-current",
};

type AlertVariant = keyof typeof alertVariantClasses;

function Alert({
  className,
  variant = "default",
  ...props
}: ComponentProps<"div"> & { variant?: AlertVariant }) {
  return (
    <div
      data-slot="alert"
      role="alert"
      className={cn(alertBase, alertVariantClasses[variant], className)}
      {...props}
    />
  );
}

function AlertDescription({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      data-slot="alert-description"
      className={cn(
        "text-sm text-balance text-muted-foreground md:text-pretty [&_a]:underline [&_a]:underline-offset-3 [&_a]:hover:text-foreground [&_p:not(:last-child)]:mb-4",
        className,
      )}
      {...props}
    />
  );
}

export { Alert, AlertDescription };
