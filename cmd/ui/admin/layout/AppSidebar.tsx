import type { LucideIcon } from "lucide-react";
import { AudioLines, Boxes, Cpu, FileJson, FolderKanban, KeyRound, LayoutDashboard, Medal, Mic2, PackageCheck, PawPrint, ServerCog, ShieldCheck, Workflow } from "lucide-react";
import { NavLink } from "react-router-dom";

import { buttonVariants } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/components/ui/utils";

const linkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    "h-11 w-full justify-start gap-3 rounded-xl px-4 text-sm font-medium transition-colors",
    isActive
      ? "bg-primary text-primary-foreground shadow-sm hover:bg-primary/90 hover:text-primary-foreground"
      : "text-muted-foreground hover:bg-muted hover:text-foreground",
  );

const childLinkClass = ({ isActive }: { isActive: boolean }) =>
  cn(
    "h-10 w-full justify-start gap-3 rounded-lg px-3 text-sm transition-colors",
    isActive
      ? "bg-primary text-primary-foreground shadow-sm hover:bg-primary/90 hover:text-primary-foreground"
      : "text-muted-foreground hover:bg-muted hover:text-foreground",
  );

type NavSection = {
  items: Array<{
    href: string;
    icon: LucideIcon;
    label: string;
  }>;
  label: string;
};

const sections: NavSection[] = [
  {
    label: "Peers",
    items: [
      { href: "/peers", icon: Boxes, label: "Peers" },
      { href: "/firmwares", icon: PackageCheck, label: "Firmwares" },
    ],
  },
  {
    label: "Providers",
    items: [
      { href: "/providers/credentials", icon: KeyRound, label: "Credentials" },
      { href: "/providers/openai-tenants", icon: ServerCog, label: "OpenAI Tenants" },
      { href: "/providers/gemini-tenants", icon: ServerCog, label: "Gemini Tenants" },
      { href: "/providers/dashscope-tenants", icon: ServerCog, label: "DashScope Tenants" },
      { href: "/providers/minimax-tenants", icon: AudioLines, label: "MiniMax Tenants" },
      { href: "/providers/volc-tenants", icon: AudioLines, label: "Volcengine Tenants" },
    ],
  },
  {
    label: "AI",
    items: [
      { href: "/ai/voices", icon: Mic2, label: "Voices" },
      { href: "/ai/models", icon: Cpu, label: "Models" },
      { href: "/ai/workflows", icon: Workflow, label: "Workflows" },
      { href: "/ai/workspaces", icon: FolderKanban, label: "Workspaces" },
    ],
  },
  {
    label: "Business",
    items: [
      { href: "/business/pet-species", icon: PawPrint, label: "Pet Species" },
      { href: "/business/badges", icon: Medal, label: "Badges" },
    ],
  },
  {
    label: "Settings",
    items: [
      { href: "/resources", icon: FileJson, label: "Resources" },
      { href: "/settings/acl", icon: ShieldCheck, label: "Access Control" },
    ],
  },
];

export function AppSidebar(): JSX.Element {
  return (
    <aside className="border-r bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
      <div className="sticky top-0 flex h-screen w-[248px] flex-col">
        <div className="px-6 py-6">
          <Card className="rounded-2xl bg-muted/30">
            <CardContent className="p-4">
              <div className="text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">GizClaw</div>
              <div className="mt-1 text-lg font-semibold tracking-tight text-foreground">Admin Console</div>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">Overview, peers, providers, and AI resource surfaces.</p>
            </CardContent>
          </Card>
        </div>

        <nav className="flex flex-1 flex-col gap-4 px-3">
          <NavLink className={({ isActive }) => cn(buttonVariants({ variant: "ghost" }), linkClass({ isActive }))} end to="/overview">
            <LayoutDashboard className="size-4" />
            Overview
          </NavLink>
          {sections.map((section) => (
            <div className="space-y-2" key={section.label}>
              <div className="px-4 text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">{section.label}</div>
              <div className="ml-4 space-y-1 border-l pl-3">
                {section.items.map((item) => (
                  <NavLink
                    className={({ isActive }) => cn(buttonVariants({ variant: "ghost" }), childLinkClass({ isActive }))}
                    end
                    key={item.href}
                    to={item.href}
                  >
                    <item.icon className="size-4" />
                    {item.label}
                  </NavLink>
                ))}
              </div>
            </div>
          ))}
        </nav>

        <div className="px-6 pb-6 pt-4">
          <Card className="rounded-xl bg-muted/20 shadow-none">
            <CardContent className="px-4 py-3 text-xs leading-5 text-muted-foreground">
              Grouped navigation keeps providers and AI resources one level below the main console.
            </CardContent>
          </Card>
        </div>
      </div>
    </aside>
  );
}
