import { useEffect, useState } from "react";
import { Braces, Check, Copy, Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { parseConfig, type ConsoleConfig } from "@/lib/config";

/**
 * The configuration is the console's source of truth, so it is shown as
 * editable JSON: read it, copy it, download it, or apply an edited version.
 */
export function ConfigDialog({
  open,
  text,
  onOpenChange,
  onApply,
}: {
  open: boolean;
  text: string;
  onOpenChange: (open: boolean) => void;
  onApply: (config: ConsoleConfig, text: string) => void;
}) {
  const [draft, setDraft] = useState(text);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (open) {
      setDraft(text);
      setError("");
      setCopied(false);
    }
  }, [open, text]);

  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 1500);
    return () => clearTimeout(timer);
  }, [copied]);

  const apply = () => {
    try {
      onApply(parseConfig(draft), draft);
      setError("");
      onOpenChange(false);
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    }
  };

  const format = () => {
    try {
      setDraft(`${JSON.stringify(JSON.parse(draft), null, 2)}\n`);
      setError("");
    } catch (failure) {
      setError(
        `无法格式化：${failure instanceof Error ? failure.message : String(failure)}`,
      );
    }
  };

  const download = () => {
    const url = URL.createObjectURL(
      new Blob([draft], { type: "application/json" }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = `gizclaw-console-${new Date().toISOString().slice(0, 10)}.json`;
    link.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>集群配置 JSON</DialogTitle>
          <DialogDescription>
            包含节点、Monitor Token
            与本浏览器的设备关注列表。可以直接编辑后应用，或复制、下载到别处。
          </DialogDescription>
        </DialogHeader>
        {error && (
          <Alert>
            <AlertTitle>配置无法使用</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <Textarea
          aria-label="配置 JSON"
          className="min-h-[46vh] flex-1 resize-none"
          spellCheck={false}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
        <DialogFooter>
          <span className="mr-auto text-[11px] text-muted-foreground">
            Token 就在这段文本里，复制或下载时请注意保管。
          </span>
          <Button variant="outline" onClick={format}>
            <Braces size={14} /> 格式化
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              void navigator.clipboard?.writeText(draft);
              setCopied(true);
            }}
          >
            {copied ? <Check size={14} /> : <Copy size={14} />}
            {copied ? "已复制" : "复制"}
          </Button>
          <Button variant="outline" onClick={download}>
            <Download size={14} /> 下载
          </Button>
          <Button onClick={apply} disabled={draft.trim() === ""}>
            应用编辑
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
