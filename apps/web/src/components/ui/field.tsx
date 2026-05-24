import type { ComponentProps } from "react";

import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

function Field({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      role="group"
      data-slot="field"
      className={cn(
        "group/field flex w-full flex-col gap-2 data-[invalid=true]:text-destructive",
        className,
      )}
      {...props}
    />
  );
}

function FieldLabel({ className, ...props }: ComponentProps<typeof Label>) {
  return (
    <Label
      data-slot="field-label"
      className={cn(
        "group/field-label flex w-fit gap-2 leading-snug group-data-[disabled=true]/field:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

function FieldDescription({ className, ...props }: ComponentProps<"p">) {
  return (
    <p
      data-slot="field-description"
      className={cn(
        "text-left text-sm leading-normal font-normal text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}

function FieldError({ className, children, ...props }: ComponentProps<"div">) {
  if (!children) return null;
  return (
    <div
      role="alert"
      data-slot="field-error"
      className={cn("text-sm font-normal text-destructive", className)}
      {...props}
    >
      {children}
    </div>
  );
}

export { Field, FieldDescription, FieldError, FieldLabel };
