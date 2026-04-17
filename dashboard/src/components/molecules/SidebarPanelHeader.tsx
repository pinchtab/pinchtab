import React from "react";

interface SidebarPanelHeaderProps {
  eyebrow?: React.ReactNode;
  title?: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
  eyebrowClassName?: string;
  titleClassName?: string;
  descriptionClassName?: string;
}

function joinClasses(...classes: Array<string | undefined | false>) {
  return classes.filter(Boolean).join(" ");
}

export default function SidebarPanelHeader({
  eyebrow,
  title,
  description,
  actions,
  className = "",
  eyebrowClassName = "",
  titleClassName = "",
  descriptionClassName = "",
}: SidebarPanelHeaderProps) {
  return (
    <div className={joinClasses("flex items-start gap-3", className)}>
      <div className="min-w-0 flex-1">
        {eyebrow && (
          <div
            className={joinClasses(
              "dashboard-section-label mb-1",
              eyebrowClassName,
            )}
          >
            {eyebrow}
          </div>
        )}
        {title && (
          <div
            className={joinClasses(
              "text-lg font-semibold text-text-primary",
              titleClassName,
            )}
          >
            {title}
          </div>
        )}
        {description && (
          <div
            className={joinClasses(
              "mt-2 text-xs leading-5 text-text-muted",
              descriptionClassName,
            )}
          >
            {description}
          </div>
        )}
      </div>
      {actions && <div className="shrink-0">{actions}</div>}
    </div>
  );
}
