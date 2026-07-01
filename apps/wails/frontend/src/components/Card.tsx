import type { PropsWithChildren, ReactNode } from "react";

export function Card({ children }: PropsWithChildren) {
  return <section className="card">{children}</section>;
}

export function CardHeader({ title, action }: { action?: ReactNode; title: string }) {
  return (
    <header className="card-header">
      <div className="card-title">{title}</div>
      {action}
    </header>
  );
}

export function CardBody({ children }: PropsWithChildren) {
  return <div className="card-body">{children}</div>;
}
