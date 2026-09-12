import { lazy, Suspense, useState } from "react";
import { MessageCircle } from "lucide-react";

import { Button } from "@/components/ui/button";

import type { AssistantPanelProps } from "./assistant-panel";

// The agent, the model client and assistant-ui load only when the panel opens.
const AssistantPanel = lazy(() => import("./assistant-panel"));

export type AssistantButtonProps = Omit<AssistantPanelProps, "onClose">;

/**
 * The floating chat entry. Once opened, the panel stays mounted while hidden
 * so the conversation survives closing and reopening it.
 */
export function AssistantButton(props: AssistantButtonProps) {
  const [open, setOpen] = useState(false);
  const [loaded, setLoaded] = useState(false);
  return (
    <div className="fixed right-4 bottom-4 z-40 flex flex-col items-end gap-3">
      {loaded && (
        <div hidden={!open}>
          <Suspense
            fallback={
              <div className="rounded-lg border bg-card p-4 text-xs text-muted-foreground shadow-xl">
                正在加载诊断助手…
              </div>
            }
          >
            <AssistantPanel {...props} onClose={() => setOpen(false)} />
          </Suspense>
        </div>
      )}
      <Button
        aria-label={open ? "收起诊断助手" : "打开诊断助手"}
        aria-expanded={open}
        className="h-11 w-11 rounded-full p-0 shadow-lg"
        onClick={() => {
          setLoaded(true);
          setOpen(!open);
        }}
      >
        <MessageCircle size={20} />
      </Button>
    </div>
  );
}
