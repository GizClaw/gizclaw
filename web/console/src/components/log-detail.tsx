import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { summarize, type LogRecord } from "@/lib/log-query";

/** Every field of one record, each one clickable as a filter. */
export function LogRecordDetail({
  record,
  onFilter,
  onClose,
}: {
  record: LogRecord | undefined;
  onFilter: (clause: string) => void;
  onClose: () => void;
}) {
  if (!record) {
    return (
      <Card className="h-fit">
        <CardHeader>
          <CardTitle>记录详情</CardTitle>
          <CardDescription>
            选中左侧任意一条记录，这里显示它的全部结构化字段；点字段即可加入过滤。
          </CardDescription>
        </CardHeader>
      </Card>
    );
  }
  const entries = Object.entries(record.fields ?? {}).sort(([a], [b]) =>
    a.localeCompare(b),
  );
  return (
    <Card className="h-fit">
      <CardHeader>
        <CardTitle>{record.level}</CardTitle>
        <CardDescription>
          {new Date(record.time).toLocaleString()} · {record.nodeName}
        </CardDescription>
        <CardAction>
          <Button
            variant="ghost"
            size="icon"
            aria-label="关闭详情"
            onClick={onClose}
          >
            <X size={14} />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {summarize(record) !== record.message && (
          <p className="text-xs break-words text-foreground">
            {summarize(record)}
          </p>
        )}
        <p className="font-mono text-xs break-words text-foreground">
          {record.message}
        </p>
        {record.error && (
          <p className="font-mono text-xs break-words text-destructive">
            {record.error}
          </p>
        )}
        {record.peer_public_key && (
          <Row
            label="peer_public_key"
            value={record.peer_public_key}
            onFilter={() =>
              onFilter(`peer_public_key:${record.peer_public_key}`)
            }
          />
        )}
        {entries.length === 0 ? (
          <p className="text-[11px] text-muted-foreground">
            这条记录没有结构化字段。
          </p>
        ) : (
          <dl className="flex flex-col gap-2">
            {entries.map(([key, value]) => (
              <Row
                key={key}
                label={key}
                value={value}
                onFilter={() => onFilter(`${key}:${value}`)}
              />
            ))}
          </dl>
        )}
        {record.fields?.request_id && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => onFilter(`request_id:${record.fields?.request_id}`)}
          >
            追踪这个请求的全部记录
          </Button>
        )}
      </CardContent>
    </Card>
  );
}

function Row({
  label,
  value,
  onFilter,
}: {
  label: string;
  value: string;
  onFilter: () => void;
}) {
  return (
    <div className="grid grid-cols-[110px_minmax(0,1fr)] items-start gap-2 border-b border-border pb-2 last:border-0">
      <dt className="truncate text-[11px] text-muted-foreground">{label}</dt>
      <dd className="m-0 min-w-0">
        <button
          onClick={onFilter}
          title="加入过滤条件"
          className="w-full text-left font-mono text-[11px] break-words hover:text-primary"
        >
          {value}
        </button>
      </dd>
    </div>
  );
}
