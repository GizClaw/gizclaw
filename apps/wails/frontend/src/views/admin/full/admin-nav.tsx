import {
  AudioLines,
  Boxes,
  ContactRound,
  Cpu,
  FileJson,
  FolderKanban,
  KeyRound,
  LayoutDashboard,
  Mic2,
  PackageCheck,
  ServerCog,
  UsersRound,
  Workflow,
} from "lucide-react";

import type { DashboardNavGroup } from "@/dashboard";

export const adminNavGroups: Array<DashboardNavGroup<string>> = [
  {
    items: [{ id: "/overview", icon: LayoutDashboard, label: "Overview" }],
  },
  {
    label: "Peers",
    items: [
      { id: "/peers", icon: Boxes, label: "Peers" },
      { id: "/firmwares", icon: PackageCheck, label: "Firmwares" },
    ],
  },
  {
    label: "Providers",
    items: [
      { id: "/providers/credentials", icon: KeyRound, label: "Credentials" },
      { id: "/providers/openai-tenants", icon: ServerCog, label: "OpenAI Tenants" },
      { id: "/providers/gemini-tenants", icon: ServerCog, label: "Gemini Tenants" },
      { id: "/providers/dashscope-tenants", icon: ServerCog, label: "DashScope Tenants" },
      { id: "/providers/deepseek-tenants", icon: ServerCog, label: "DeepSeek Tenants" },
      { id: "/providers/minimax-tenants", icon: AudioLines, label: "MiniMax Tenants" },
      { id: "/providers/volc-tenants", icon: AudioLines, label: "Volcengine Tenants" },
    ],
  },
  {
    label: "AI",
    items: [
      { id: "/ai/voices", icon: Mic2, label: "Voices" },
      { id: "/ai/models", icon: Cpu, label: "Models" },
      { id: "/ai/workflows", icon: Workflow, label: "Workflows" },
      { id: "/ai/workspaces", icon: FolderKanban, label: "Workspaces" },
    ],
  },
  {
    label: "Social",
    items: [
      { id: "/social/contacts", icon: ContactRound, label: "Contacts" },
      { id: "/social/friends", icon: UsersRound, label: "Friends" },
      { id: "/social/friend-groups", icon: UsersRound, label: "Friend Groups" },
    ],
  },
  {
    label: "Settings",
	items: [{ id: "/resources", icon: FileJson, label: "Resources" }],
  },
];

export function adminNavTitle(pathname: string): string {
  return matchAdminNavItem(pathname)?.label ?? "Admin Console";
}

export function adminActiveNavID(pathname: string): string {
  return matchAdminNavItem(pathname)?.id ?? "/overview";
}

function matchAdminNavItem(pathname: string): { id: string; label: string } | undefined {
  const items = adminNavGroups.flatMap((group) => group.items);
  return items
    .filter((item) => pathname === item.id || pathname.startsWith(`${item.id}/`))
    .sort((left, right) => right.id.length - left.id.length)[0];
}
