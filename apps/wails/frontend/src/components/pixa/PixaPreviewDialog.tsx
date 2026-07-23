import { Pause, Play, RotateCcw } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { JSX } from "react";
import {
  drawPixaFrame,
  pixaClipFrameIndex,
  selectPixaClip,
  type PixaAsset,
  type PixaClip,
} from "@gizclaw/pixa";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/components/ui/utils";

type PixaPreviewDialogProps = {
  asset: PixaAsset;
  confirmLabel?: string;
  description?: string;
  onClose(): void;
  onConfirm?: () => void;
  title: string;
};

export function PixaPreviewDialog({
  asset,
  confirmLabel = "Use Pixa",
  description,
  onClose,
  onConfirm,
  title,
}: PixaPreviewDialogProps): JSX.Element {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [clipName, setClipName] = useState(
    () => selectPixaClip(asset)?.name ?? "",
  );
  const [elapsedMs, setElapsedMs] = useState(0);
  const [playing, setPlaying] = useState(true);
  const [loop, setLoop] = useState(true);
  const [renderError, setRenderError] = useState("");
  const selectedClip = useMemo(
    () => selectPixaClip(asset, clipName),
    [asset, clipName],
  );
  const frameIndex =
    selectedClip == null
      ? 0
      : pixaClipFrameIndex({ ...selectedClip, loop }, elapsedMs);

  useEffect(() => {
    setClipName(selectPixaClip(asset)?.name ?? "");
    setElapsedMs(0);
    setPlaying(true);
    setRenderError("");
  }, [asset]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas == null || selectedClip == null) {
      return;
    }
    const ctx = canvas.getContext("2d");
    if (ctx == null) {
      setRenderError("Canvas rendering is unavailable.");
      return;
    }
    try {
      drawPixaFrame(ctx, asset, frameIndex);
      setRenderError("");
    } catch (error) {
      setRenderError(error instanceof Error ? error.message : String(error));
      ctx.clearRect(0, 0, canvas.width, canvas.height);
    }
  }, [asset, frameIndex, selectedClip]);

  useEffect(() => {
    if (!playing || selectedClip == null) {
      return;
    }
    const startedAt = performance.now() - elapsedMs;
    let raf = 0;
    const tick = (now: number): void => {
      const duration =
        selectedClip.totalDurationMs > 0 ? selectedClip.totalDurationMs : 120;
      const nextElapsed = loop
        ? (now - startedAt) % duration
        : Math.min(now - startedAt, duration - 1);
      setElapsedMs(nextElapsed);
      if (loop || nextElapsed < duration - 1) {
        raf = requestAnimationFrame(tick);
      } else {
        setPlaying(false);
      }
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [elapsedMs, loop, playing, selectedClip]);

  const restart = (): void => {
    setElapsedMs(0);
    setPlaying(true);
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
    >
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description != null ? (
            <DialogDescription>{description}</DialogDescription>
          ) : null}
        </DialogHeader>
        <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_12rem]">
          <div className="flex min-h-64 items-center justify-center rounded-md border bg-muted/30 p-4">
            <canvas
              ref={canvasRef}
              className={cn(
                "max-h-56 max-w-full rounded border bg-background [image-rendering:pixelated]",
                renderError !== "" && "opacity-40",
              )}
              style={{
                aspectRatio: `${asset.canvas.width} / ${asset.canvas.height}`,
              }}
            />
          </div>
          <div className="flex flex-col gap-3">
            <Select
              disabled={asset.clips.length === 0}
              onValueChange={(value) => {
                setClipName(value);
                setElapsedMs(0);
                setPlaying(true);
              }}
              value={selectedClip?.name ?? ""}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {asset.clips.map((clip) => (
                  <SelectItem
                    key={`${clip.name}:${clip.firstFrame}`}
                    value={clip.name}
                  >
                    {clip.name || `clip-${clip.firstFrame}`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="flex gap-2">
              <Button
                onClick={() => setPlaying((value) => !value)}
                size="icon"
                type="button"
                variant="outline"
              >
                {playing ? (
                  <Pause className="size-4" />
                ) : (
                  <Play className="size-4" />
                )}
              </Button>
              <Button
                onClick={restart}
                size="icon"
                type="button"
                variant="outline"
              >
                <RotateCcw className="size-4" />
              </Button>
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                checked={loop}
                className="size-4 accent-primary"
                onChange={(event) => setLoop(event.target.checked)}
                type="checkbox"
              />
              Loop
            </label>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm text-muted-foreground">
              <dt>Canvas</dt>
              <dd>
                {asset.canvas.width} x {asset.canvas.height}
              </dd>
              <dt>Clips</dt>
              <dd>{asset.clipCount}</dd>
              <dt>Frames</dt>
              <dd>{asset.frameCount}</dd>
              <dt>Frame</dt>
              <dd>{frameIndex}</dd>
            </dl>
          </div>
        </div>
        {renderError !== "" ? (
          <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {renderError}
          </p>
        ) : null}
        <DialogFooter>
          <Button onClick={onClose} type="button" variant="outline">
            Close
          </Button>
          {onConfirm != null ? (
            <Button onClick={onConfirm} type="button">
              {confirmLabel}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export type { PixaClip };
