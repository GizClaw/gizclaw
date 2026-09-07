import { useState, type FormEvent } from "react";
import { KeyRound, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { parseConfig, sampleConfig, type ConsoleConfig } from "@/lib/config";

export function LoginPage({
  onConnect,
}: {
  onConnect: (config: ConsoleConfig, text: string, remember: boolean) => void;
}) {
  const [text, setText] = useState("");
  const [remember, setRemember] = useState(true);
  const [error, setError] = useState("");

  const submit = (event: FormEvent) => {
    event.preventDefault();
    try {
      onConnect(parseConfig(text), text, remember);
      setError("");
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : String(failure));
    }
  };

  return (
    <div className="mx-auto flex min-h-screen w-full max-w-3xl flex-col justify-center gap-6 px-6 py-12">
      <div>
        <div className="mb-2 text-[9px] tracking-[0.2em] text-muted-foreground">
          GIZCLAW CONSOLE
        </div>
        <h1 className="font-serif text-[30px] font-medium tracking-tight">
          接入你的集群
        </h1>
        <p className="mt-1.5 text-xs text-muted-foreground">
          粘贴一份配置，列出每个 Server / Edge 节点的地址和它自己的 Monitor
          Token。浏览器直接连接这些节点，配置不会上传到任何服务。
        </p>
      </div>
      {error && (
        <Alert>
          <AlertTitle>配置无法使用</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <form onSubmit={submit}>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <KeyRound size={16} className="text-primary" /> 集群配置
            </CardTitle>
            <CardDescription>
              JSON 格式。servers 可以是数组，也可以是以节点地址为键的对象。
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <Label htmlFor="config">配置内容</Label>
            <Textarea
              id="config"
              value={text}
              spellCheck={false}
              onChange={(event) => setText(event.target.value)}
              placeholder={sampleConfig}
            />
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <input
                type="checkbox"
                checked={remember}
                onChange={(event) => setRemember(event.target.checked)}
              />
              在此浏览器加密保存（IndexedDB + 不可导出的 Web Crypto 密钥）
            </label>
          </CardContent>
          <CardFooter className="justify-between gap-4">
            <span className="flex items-center gap-2 text-[11px] text-muted-foreground">
              <ShieldCheck size={14} />{" "}
              同源脚本可以使用该密钥，这不是操作系统钥匙串。
            </span>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setText(sampleConfig)}
              >
                填入示例
              </Button>
              <Button type="submit" disabled={text.trim() === ""}>
                接入集群
              </Button>
            </div>
          </CardFooter>
        </Card>
      </form>
    </div>
  );
}
