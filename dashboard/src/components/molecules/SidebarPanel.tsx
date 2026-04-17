import React from "react";

type SidebarSurface = "none" | "surface" | "panel";
type SidebarWidth = "auto" | "narrow" | "wide";
type SidebarChrome = "none" | "sidebar";
type SidebarPadding = "none" | "sm" | "md" | "lg";

interface SidebarPanelProps {
  header?: React.ReactNode;
  footer?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
  headerClassName?: string;
  contentClassName?: string;
  footerClassName?: string;
  as?: "aside" | "div" | "section";
  scrollContent?: boolean;
  surface?: SidebarSurface;
  width?: SidebarWidth;
  chrome?: SidebarChrome;
  headerPadding?: SidebarPadding;
  contentPadding?: SidebarPadding;
  footerPadding?: SidebarPadding;
}

function joinClasses(...classes: Array<string | undefined | false>) {
  return classes.filter(Boolean).join(" ");
}

function paddingClass(size: SidebarPadding): string {
  switch (size) {
    case "sm":
      return "p-3";
    case "md":
      return "px-4 py-3";
    case "lg":
      return "px-4 py-4";
    default:
      return "";
  }
}

function surfaceClass(surface: SidebarSurface): string {
  switch (surface) {
    case "surface":
      return "bg-bg-surface";
    case "panel":
      return "dashboard-panel";
    default:
      return "";
  }
}

function widthClass(width: SidebarWidth): string {
  switch (width) {
    case "narrow":
      return "w-full shrink-0 lg:w-72";
    case "wide":
      return "w-full shrink-0 lg:w-80";
    default:
      return "";
  }
}

function chromeClass(chrome: SidebarChrome): string {
  switch (chrome) {
    case "sidebar":
      return "border-b border-border-subtle lg:border-b-0 lg:border-r";
    default:
      return "";
  }
}

export default function SidebarPanel({
  header,
  footer,
  children,
  className = "",
  headerClassName = "",
  contentClassName = "",
  footerClassName = "",
  as: Tag = "div",
  scrollContent = true,
  surface = "none",
  width = "auto",
  chrome = "none",
  headerPadding = "none",
  contentPadding = "none",
  footerPadding = "none",
}: SidebarPanelProps) {
  return (
    <Tag
      className={joinClasses(
        "flex min-h-0 flex-col overflow-hidden",
        surfaceClass(surface),
        widthClass(width),
        chromeClass(chrome),
        className,
      )}
    >
      {header && (
        <div
          className={joinClasses(
            "shrink-0 border-b border-border-subtle",
            paddingClass(headerPadding),
            headerClassName,
          )}
        >
          {header}
        </div>
      )}
      <div
        className={joinClasses(
          scrollContent ? "min-h-0 flex-1 overflow-auto" : "min-h-0 flex-1",
          paddingClass(contentPadding),
          contentClassName,
        )}
      >
        {children}
      </div>
      {footer && (
        <div
          className={joinClasses(
            "shrink-0 border-t border-border-subtle",
            paddingClass(footerPadding),
            footerClassName,
          )}
        >
          {footer}
        </div>
      )}
    </Tag>
  );
}
