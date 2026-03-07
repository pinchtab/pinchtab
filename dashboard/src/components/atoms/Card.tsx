import type { HTMLAttributes } from "react";

interface Props extends HTMLAttributes<HTMLDivElement> {
  hover?: boolean;
  selected?: boolean;
}

export default function Card({
  hover = false,
  selected = false,
  className = "",
  children,
  ...props
}: Props) {
  return (
    <div
      className={`dashboard-panel ${
        selected ? "dashboard-panel-selected border-primary" : ""
      } ${hover ? "dashboard-panel-hover cursor-pointer" : ""} ${className}`}
      {...props}
    >
      {children}
    </div>
  );
}
