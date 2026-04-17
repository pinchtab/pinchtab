import React from "react";

interface SidebarPanelFooterActionsProps {
  children: React.ReactNode;
  className?: string;
  align?: "start" | "end" | "between";
}

function joinClasses(...classes: Array<string | undefined | false>) {
  return classes.filter(Boolean).join(" ");
}

function alignClass(align: SidebarPanelFooterActionsProps["align"]): string {
  switch (align) {
    case "end":
      return "justify-end";
    case "between":
      return "justify-between";
    default:
      return "";
  }
}

export default function SidebarPanelFooterActions({
  children,
  className = "",
  align = "start",
}: SidebarPanelFooterActionsProps) {
  return (
    <div
      className={joinClasses("flex gap-2 p-4", alignClass(align), className)}
    >
      {children}
    </div>
  );
}
