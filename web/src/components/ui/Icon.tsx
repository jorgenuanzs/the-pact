import type { ReactNode, SVGProps } from "react";

import { cx } from "./utils";

export type IconName =
  | "access"
  | "activity"
  | "arrowRight"
  | "close"
  | "computer"
  | "context"
  | "empty"
  | "folder"
  | "hash"
  | "home"
  | "logout"
  | "mentions"
  | "menu"
  | "people"
  | "play"
  | "plus"
  | "repository"
  | "search"
  | "server"
  | "settings";

export interface IconProps extends Omit<SVGProps<SVGSVGElement>, "name"> {
  name: IconName;
  size?: "sm" | "md" | "lg";
}

const paths: Record<IconName, ReactNode> = {
  access: <><path d="M12 3.25 18.5 6v4.75c0 4.15-2.65 7.65-6.5 9-3.85-1.35-6.5-4.85-6.5-9V6L12 3.25Z" /><path d="m9.25 11.75 1.75 1.75 3.75-4" /></>,
  activity: <><path d="M5 6.5h14M5 12h14M5 17.5h9" /><circle cx="3" cy="6.5" r=".6" fill="currentColor" stroke="none" /><circle cx="3" cy="12" r=".6" fill="currentColor" stroke="none" /><circle cx="3" cy="17.5" r=".6" fill="currentColor" stroke="none" /></>,
  arrowRight: <><path d="M4.5 12h15M14 6.5l5.5 5.5-5.5 5.5" /></>,
  close: <><path d="m6.5 6.5 11 11M17.5 6.5l-11 11" /></>,
  computer: <><rect x="3.5" y="4.5" width="17" height="12" rx="1.5" /><path d="M8 20h8M12 16.5V20" /></>,
  context: <><path d="M5 4.5h10.25A3.75 3.75 0 0 1 19 8.25V19.5H8.75A3.75 3.75 0 0 1 5 15.75V4.5Z" /><path d="M8.5 8.5h7M8.5 12h7M8.5 15.5h4.5" /></>,
  empty: <><path d="M12 3.5 20.5 12 12 20.5 3.5 12 12 3.5Z" /><path d="M8.5 12h7" /></>,
  folder: <><path d="M3.5 6.5h6l2 2h9v10.25A1.75 1.75 0 0 1 18.75 20H5.25a1.75 1.75 0 0 1-1.75-1.75V6.5Z" /><path d="M3.5 10h17" /></>,
  hash: <><path d="m9 3.5-2 17M17 3.5l-2 17M4.5 9h16M3.5 15h16" /></>,
  home: <><path d="m4 10.5 8-6.5 8 6.5" /><path d="M6.5 9v10.5h11V9M10 19.5v-6h4v6" /></>,
  logout: <><path d="M10 5H5v14h5M14.5 8 18.5 12l-4 4M8 12h10" /></>,
  mentions: <><path d="M16.75 15.5c-1.2 1.35-2.75 2-4.75 2a5.5 5.5 0 1 1 5.5-5.5v1.25a2.25 2.25 0 0 0 4.5 0V12a10 10 0 1 0-3.1 7.25" /><circle cx="12" cy="12" r="2.75" /></>,
  menu: <><path d="M4 6.5h16M4 12h16M4 17.5h16" /></>,
  people: <><circle cx="9" cy="8" r="3" /><path d="M3.75 19c.45-3.3 2.2-5 5.25-5s4.8 1.7 5.25 5M15.5 5.5a3 3 0 0 1 0 5.8M16 14c2.45.2 3.85 1.85 4.25 5" /></>,
  play: <><path d="m8 5.25 10 6.75L8 18.75V5.25Z" /></>,
  plus: <><path d="M12 4.5v15M4.5 12h15" /></>,
  repository: <><circle cx="7" cy="5" r="2" /><circle cx="17" cy="7" r="2" /><circle cx="7" cy="19" r="2" /><path d="M7 7v10M9 7h3a5 5 0 0 1 5 5v3" /></>,
  search: <><circle cx="10.5" cy="10.5" r="6" /><path d="m15 15 4.5 4.5" /></>,
  server: <><rect x="4" y="3.5" width="16" height="7" rx="1.5" /><rect x="4" y="13.5" width="16" height="7" rx="1.5" /><path d="M8 7h.01M8 17h.01M11 7h5M11 17h5" /></>,
  settings: <><circle cx="12" cy="12" r="3" /><path d="M19 13.5a7.5 7.5 0 0 0 0-3l2-1.25-2-3.5-2.25 1a8.25 8.25 0 0 0-2.5-1.5L14 2.75h-4l-.25 2.5a8.25 8.25 0 0 0-2.5 1.5L5 5.75l-2 3.5 2 1.25a7.5 7.5 0 0 0 0 3l-2 1.25 2 3.5 2.25-1a8.25 8.25 0 0 0 2.5 1.5l.25 2.5h4l.25-2.5a8.25 8.25 0 0 0 2.5-1.5l2.25 1 2-3.5-2-1.25Z" /></>,
};

export function Icon({ name, size = "md", className, ...props }: IconProps) {
  return (
    <svg
      aria-hidden="true"
      className={cx("pact-icon", className)}
      data-size={size}
      fill="none"
      focusable="false"
      viewBox="0 0 24 24"
      {...props}
    >
      {paths[name]}
    </svg>
  );
}
