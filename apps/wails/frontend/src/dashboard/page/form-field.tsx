import * as React from "react";

import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { cn } from "@/components/ui/utils";

interface FormFieldProps extends React.HTMLAttributes<HTMLDivElement> {
  description?: string;
  htmlFor?: string;
  label: string;
}

const FormField = React.forwardRef<HTMLDivElement, FormFieldProps>(
  ({ children, className, description, htmlFor, label, ...props }, ref) => (
    <Field ref={ref} className={cn("gap-2 rounded-lg border bg-muted/20 p-4", className)} {...props}>
      <div className="flex flex-col gap-1">
        <FieldLabel htmlFor={htmlFor}>{label}</FieldLabel>
        {description ? <FieldDescription>{description}</FieldDescription> : null}
      </div>
      {children}
    </Field>
  ),
);
FormField.displayName = "FormField";

export { FormField };
