import type { ReactNode } from "react";

type ManageListItemProps = {
  className: string;
  description: ReactNode;
  icon: ReactNode;
  iconClassName: string;
  title: ReactNode;
  trailing: ReactNode;
} & (
  | { as: "section" }
  | {
      as: "button";
      onClick(): void;
    }
);

export function ManageListItem(props: ManageListItemProps) {
  const content = (
    <>
      <span className={`manage-list-item-icon ${props.iconClassName}`}>
        {props.icon}
      </span>
      <span className="manage-list-item-copy">
        <strong>{props.title}</strong>
        <small>{props.description}</small>
      </span>
      <span className="manage-list-item-trailing">{props.trailing}</span>
    </>
  );

  if (props.as === "button") {
    return (
      <button
        className={`manage-list-item manage-list-item-button ${props.className}`}
        onClick={props.onClick}
        type="button"
      >
        {content}
      </button>
    );
  }

  return (
    <section className={`manage-list-item ${props.className}`}>
      {content}
    </section>
  );
}
