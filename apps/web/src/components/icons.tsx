import type { SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function Icon({ size = 18, children, ...props }: IconProps) {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height={size}
      viewBox="0 0 24 24"
      width={size}
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      {...props}
    >
      {children}
    </svg>
  );
}

export const Icons = {
  Activity: (props: IconProps) => <Icon {...props}><path d="M3 12h4l2.5-6 5 12 2.5-6h4" /></Icon>,
  Arrow: (props: IconProps) => <Icon {...props}><path d="M5 12h14M14 7l5 5-5 5" /></Icon>,
  Bell: (props: IconProps) => <Icon {...props}><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4" /></Icon>,
  Branch: (props: IconProps) => <Icon {...props}><circle cx="6" cy="5" r="2"/><circle cx="18" cy="7" r="2"/><circle cx="6" cy="19" r="2"/><path d="M6 7v10M8 7h4a6 6 0 0 1 6 6v-4"/></Icon>,
  Chevron: (props: IconProps) => <Icon {...props}><path d="m9 18 6-6-6-6" /></Icon>,
  Code: (props: IconProps) => <Icon {...props}><path d="m8 9-3 3 3 3m8-6 3 3-3 3m-2-9-4 12" /></Icon>,
  Compass: (props: IconProps) => <Icon {...props}><circle cx="12" cy="12" r="9"/><path d="m15 9-2 4-4 2 2-4 4-2Z"/></Icon>,
  GitPull: (props: IconProps) => <Icon {...props}><circle cx="6" cy="5" r="2"/><circle cx="18" cy="19" r="2"/><path d="M6 7v12M18 17V9a4 4 0 0 0-4-4H9m2-3L8 5l3 3"/></Icon>,
  Home: (props: IconProps) => <Icon {...props}><path d="m3 11 9-8 9 8M5 10v10h14V10M9 20v-6h6v6"/></Icon>,
  Menu: (props: IconProps) => <Icon {...props}><path d="M4 7h16M4 12h16M4 17h16"/></Icon>,
  Plus: (props: IconProps) => <Icon {...props}><path d="M12 5v14M5 12h14"/></Icon>,
  Search: (props: IconProps) => <Icon {...props}><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></Icon>,
  Settings: (props: IconProps) => <Icon {...props}><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1a1.7 1.7 0 0 0 1.9.3 1.7 1.7 0 0 0 1-1.6v-.2h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z"/></Icon>,
  Spark: (props: IconProps) => <Icon {...props}><path d="m12 3 1.4 4.1L17 9l-3.6 1.9L12 15l-1.4-4.1L7 9l3.6-1.9L12 3ZM5 15l.8 2.2L8 18l-2.2.8L5 21l-.8-2.2L2 18l2.2-.8L5 15Zm14-2 .8 2.2L22 16l-2.2.8L19 19l-.8-2.2L16 16l2.2-.8L19 13Z"/></Icon>,
};
