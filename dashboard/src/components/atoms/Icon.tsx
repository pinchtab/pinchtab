import type { SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function icon(paths: string[], displayName: string) {
  function Icon({ size = 18, ...props }: IconProps) {
    return (
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.5}
        strokeLinecap="round"
        strokeLinejoin="round"
        {...props}
      >
        {paths.map((d, i) => (
          <path key={i} d={d} />
        ))}
      </svg>
    );
  }
  Icon.displayName = displayName;
  return Icon;
}

export const IconRobot = icon(
  [
    "M6 6a2 2 0 0 1 2 -2h8a2 2 0 0 1 2 2v4a2 2 0 0 1 -2 2h-8a2 2 0 0 1 -2 -2l0 -4",
    "M12 2v2",
    "M9 12v9",
    "M15 12v9",
    "M5 16l4 -2",
    "M15 14l4 2",
    "M9 18h6",
    "M10 8v.01",
    "M14 8v.01",
  ],
  "IconRobot",
);

export const IconBolt = icon(
  ["M13 3l0 7l6 0l-8 11l0 -7l-6 0l8 -11"],
  "IconBolt",
);

export const IconCompass = icon(
  [
    "M8 16l2 -6l6 -2l-2 6l-6 2",
    "M3 12a9 9 0 1 0 18 0a9 9 0 1 0 -18 0",
    "M12 3l0 2",
    "M12 19l0 2",
    "M3 12l2 0",
    "M19 12l2 0",
  ],
  "IconCompass",
);

export const IconHandClick = icon(
  [
    "M8 13v-8.5a1.5 1.5 0 0 1 3 0v7.5",
    "M11 11.5v-2a1.5 1.5 0 0 1 3 0v2.5",
    "M14 10.5a1.5 1.5 0 0 1 3 0v1.5",
    "M17 11.5a1.5 1.5 0 0 1 3 0v4.5a6 6 0 0 1 -6 6h-2h.208a6 6 0 0 1 -5.012 -2.7l-.196 -.3c-.312 -.479 -1.407 -2.388 -3.286 -5.728a1.5 1.5 0 0 1 .536 -2.022a1.867 1.867 0 0 1 2.28 .28l1.47 1.47",
    "M5 3l-1 -1",
    "M4 7h-1",
    "M14 3l1 -1",
    "M15 6h1",
  ],
  "IconHandClick",
);

export const IconKeyboard = icon(
  [
    "M2 8a2 2 0 0 1 2 -2h16a2 2 0 0 1 2 2v8a2 2 0 0 1 -2 2h-16a2 2 0 0 1 -2 -2l0 -8",
    "M6 10l0 .01",
    "M10 10l0 .01",
    "M14 10l0 .01",
    "M18 10l0 .01",
    "M6 14l0 .01",
    "M18 14l0 .01",
    "M10 14l4 .01",
  ],
  "IconKeyboard",
);

export const IconPointer = icon(
  [
    "M7.904 17.563a1.2 1.2 0 0 0 2.228 .308l2.09 -3.093l4.907 4.907a1.067 1.067 0 0 0 1.509 0l1.047 -1.047a1.067 1.067 0 0 0 0 -1.509l-4.907 -4.907l3.113 -2.09a1.2 1.2 0 0 0 -.309 -2.228l-13.582 -3.904l3.904 13.563",
  ],
  "IconPointer",
);

export const IconCamera = icon(
  [
    "M5 7h1a2 2 0 0 0 2 -2a1 1 0 0 1 1 -1h6a1 1 0 0 1 1 1a2 2 0 0 0 2 2h1a2 2 0 0 1 2 2v9a2 2 0 0 1 -2 2h-14a2 2 0 0 1 -2 -2v-9a2 2 0 0 1 2 -2",
    "M9 13a3 3 0 1 0 6 0a3 3 0 0 0 -6 0",
  ],
  "IconCamera",
);

export const IconFileText = icon(
  [
    "M14 3v4a1 1 0 0 0 1 1h4",
    "M17 21h-10a2 2 0 0 1 -2 -2v-14a2 2 0 0 1 2 -2h7l5 5v11a2 2 0 0 1 -2 2",
    "M9 9l1 0",
    "M9 13l6 0",
    "M9 17l6 0",
  ],
  "IconFileText",
);

export const IconMessageCircle = icon(
  [
    "M3 20l1.3 -3.9c-2.324 -3.437 -1.426 -7.872 2.1 -10.374c3.526 -2.501 8.59 -2.296 11.845 .48c3.255 2.777 3.695 7.266 1.029 10.501c-2.666 3.235 -7.615 4.215 -11.574 2.293l-4.7 1",
  ],
  "IconMessageCircle",
);

export const IconBrain = icon(
  [
    "M15.5 13a3.5 3.5 0 0 0 -3.5 3.5v1a3.5 3.5 0 0 0 7 0v-1.8",
    "M8.5 13a3.5 3.5 0 0 1 3.5 3.5v1a3.5 3.5 0 0 1 -7 0v-1.8",
    "M17.5 16a3.5 3.5 0 0 0 0 -7h-.5",
    "M19 9.3v-2.8a3.5 3.5 0 0 0 -7 0",
    "M6.5 16a3.5 3.5 0 0 1 0 -7h.5",
    "M5 9.3v-2.8a3.5 3.5 0 0 1 7 0v10",
  ],
  "IconBrain",
);

export const IconSearch = icon(
  ["M3 10a7 7 0 1 0 14 0a7 7 0 1 0 -14 0", "M21 21l-6 -6"],
  "IconSearch",
);

export const IconPhoto = icon(
  [
    "M15 8h.01",
    "M3 6a3 3 0 0 1 3 -3h12a3 3 0 0 1 3 3v12a3 3 0 0 1 -3 3h-12a3 3 0 0 1 -3 -3v-12",
    "M3 16l5 -5c.928 -.893 2.072 -.893 3 0l5 5",
    "M14 14l1 -1c.928 -.893 2.072 -.893 3 0l3 3",
  ],
  "IconPhoto",
);

export const IconScreenShare = icon(
  [
    "M21 12v3a1 1 0 0 1 -1 1h-16a1 1 0 0 1 -1 -1v-10a1 1 0 0 1 1 -1h9",
    "M7 20l10 0",
    "M9 16l0 4",
    "M15 16l0 4",
    "M17 4h4v4",
    "M16 9l5 -5",
  ],
  "IconScreenShare",
);
