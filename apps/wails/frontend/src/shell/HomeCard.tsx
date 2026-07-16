import type { ComponentProps, ReactNode } from "react";

type HomeCardProps = Omit<ComponentProps<"button">, "children"> & {
  copyClassName?: string;
  description: ReactNode;
  footer?: ReactNode;
  title: ReactNode;
  top: ReactNode;
};

export function HomeCard({
  className = "",
  copyClassName = "",
  description,
  footer,
  title,
  top,
  type = "button",
  ...props
}: HomeCardProps) {
  return (
    <button className={className} data-slot="home-card" type={type} {...props}>
      <span className="home-card-top">{top}</span>
      <span className={`home-card-copy ${copyClassName}`.trim()}>
        <strong>{title}</strong>
        <small>{description}</small>
      </span>
      {footer}
    </button>
  );
}
