import type { ButtonHTMLAttributes, PropsWithChildren } from "react";

type ButtonProps = PropsWithChildren<
  ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: "primary" | "secondary";
  }
>;

export function Button({ children, className = "", variant = "primary", ...props }: ButtonProps) {
  const variantClass = variant === "secondary" ? " secondary" : "";
  return (
    <button className={`button${variantClass} ${className}`.trim()} {...props}>
      {children}
    </button>
  );
}
