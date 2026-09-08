import type { ReactNode } from "react";
import {
  Activity,
  ChevronRight,
  Download,
  LogOut,
  ScrollText,
  Server,
  Smartphone,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ConsoleServer } from "@/lib/config";

export function AppShell({
  title,
  servers,
  activeId,
  page,
  breadcrumb,
  onExport,
  onLogout,
  children,
}: {
  title: string;
  servers: ConsoleServer[];
  activeId?: string;
  page: "overview" | "server" | "logs" | "peers";
  breadcrumb: string[];
  onExport: () => void;
  onLogout: () => void;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-screen flex-col md:h-screen md:flex-row md:overflow-hidden">
      <aside className="flex shrink-0 flex-wrap items-center gap-4 border-b border-border bg-secondary p-4 md:fixed md:inset-y-0 md:left-0 md:w-60 md:flex-col md:items-stretch md:gap-6 md:p-5 md:border-b-0 md:border-r">
        <div>
          <div className="flex items-center gap-2 text-lg font-semibold">
            <span className="text-2xl leading-none text-primary">✳</span>
            GizClaw
          </div>
          <div className="mt-1 hidden pl-8 text-[9px] font-medium tracking-[0.28em] text-muted-foreground md:block">
            CONSOLE
          </div>
        </div>
        <nav className="flex gap-1.5 overflow-x-auto md:flex-col">
          <div className="hidden px-2 text-[10px] tracking-[0.15em] text-muted-foreground md:block">
            {title.toUpperCase()}
          </div>
          <NavLink
            href="#/"
            active={page === "overview"}
            icon={<Activity size={15} />}
          >
            集群总览
          </NavLink>
          <NavLink
            href="#/logs"
            active={page === "logs"}
            icon={<ScrollText size={15} />}
          >
            日志查询
          </NavLink>
          <NavLink
            href="#/peers"
            active={page === "peers"}
            icon={<Smartphone size={15} />}
          >
            设备监控
          </NavLink>
          {servers.map((server) => (
            <NavLink
              key={server.id}
              href={`#/server/${encodeURIComponent(server.id)}`}
              active={activeId === server.id}
              icon={<Server size={15} />}
            >
              {server.name}
            </NavLink>
          ))}
        </nav>
        <div className="mt-auto hidden gap-3 border-t border-border pt-4 text-[11px] leading-relaxed text-muted-foreground md:grid">
          <p>
            数据来自各节点自身的 Monitor Token，浏览器直连，不经过第三方服务。
          </p>
          <Button
            variant="ghost"
            size="sm"
            className="justify-start"
            onClick={onExport}
          >
            <Download size={14} /> 配置 JSON
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="justify-start"
            onClick={onLogout}
          >
            <LogOut size={14} /> 退出并清除配置
          </Button>
        </div>
      </aside>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col md:ml-60">
        <header className="flex h-14 items-center justify-between border-b border-border px-6 text-xs text-muted-foreground">
          <span className="flex items-center gap-2">
            {breadcrumb.map((part, index) => (
              <span key={part} className="flex items-center gap-2">
                {index > 0 && <ChevronRight size={13} />}
                {part}
              </span>
            ))}
          </span>
          <Button
            variant="ghost"
            size="sm"
            className="md:hidden"
            onClick={onLogout}
          >
            <LogOut size={14} /> 退出
          </Button>
        </header>
        <main className="mx-auto flex w-full max-w-[1440px] min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-6 py-8">
          {children}
        </main>
      </div>
    </div>
  );
}

function NavLink({
  href,
  active,
  icon,
  children,
}: {
  href: string;
  active: boolean;
  icon: ReactNode;
  children: ReactNode;
}) {
  return (
    <a
      href={href}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex items-center gap-2.5 truncate rounded-md px-3 py-2.5 text-xs text-muted-foreground transition-colors hover:bg-accent/60",
        active && "bg-accent font-medium text-accent-foreground",
      )}
    >
      <span className={cn(active && "text-primary")}>{icon}</span>
      <span className="truncate">{children}</span>
    </a>
  );
}

export function PageHeading({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <div className="mb-2 text-[9px] tracking-[0.2em] text-muted-foreground">
          {eyebrow}
        </div>
        <h1 className="font-serif text-[28px] font-medium tracking-tight">
          {title}
        </h1>
        <p className="mt-1.5 text-xs text-muted-foreground">{description}</p>
      </div>
      {action}
    </div>
  );
}
