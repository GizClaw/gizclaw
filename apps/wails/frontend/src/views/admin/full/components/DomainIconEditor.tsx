import { Download, ImageIcon, Trash2, Upload } from "lucide-react";
import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { expectData, toMessage } from "@/dashboard";
import {
  deleteGameDefIcon,
  deleteWorkspaceIcon,
  downloadGameDefIcon,
  downloadWorkspaceIcon,
  uploadGameDefIcon,
  uploadWorkspaceIcon,
} from "@gizclaw/gizclaw/admin";

type IconOwner = "game-def" | "workspace";
type IconFormat = "pixa" | "png";

type Props = {
  icon?: { pixa?: string; png?: string };
  id: string;
  onChanged?: () => void | Promise<void>;
  owner: IconOwner;
};

export function DomainIconEditor({
  icon,
  id,
  onChanged,
  owner,
}: Props): JSX.Element {
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [previewURL, setPreviewURL] = useState("");

  useEffect(
    () => () => {
      if (previewURL !== "") URL.revokeObjectURL(previewURL);
    },
    [previewURL],
  );

  const run = async (
    key: string,
    action: () => Promise<void>,
  ): Promise<void> => {
    setBusy(key);
    setError("");
    try {
      await action();
      await onChanged?.();
    } catch (cause) {
      setError(toMessage(cause));
    } finally {
      setBusy("");
    }
  };

  const upload = (format: IconFormat, file: File): Promise<void> =>
    run(`upload-${format}`, async () => {
      const path = owner === "game-def" ? { id, format } : { name: id, format };
      if (owner === "game-def")
        await expectData(
          uploadGameDefIcon({
            body: file,
            path: path as { id: string; format: IconFormat },
          }),
        );
      else
        await expectData(
          uploadWorkspaceIcon({
            body: file,
            path: path as { name: string; format: IconFormat },
          }),
        );
      if (format === "png") setPreview(file);
    });

  const download = (format: IconFormat): Promise<void> =>
    run(`download-${format}`, async () => {
      const blob =
        owner === "game-def"
          ? await expectData(downloadGameDefIcon({ path: { id, format } }))
          : await expectData(
              downloadWorkspaceIcon({ path: { name: id, format } }),
            );
      if (format === "png") {
        setPreview(blob);
        return;
      }
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = `${id}.pixa`;
      anchor.click();
      URL.revokeObjectURL(url);
    });

  const remove = (format: IconFormat): Promise<void> =>
    run(`delete-${format}`, async () => {
      if (owner === "game-def")
        await expectData(deleteGameDefIcon({ path: { id, format } }));
      else
        await expectData(deleteWorkspaceIcon({ path: { name: id, format } }));
      if (format === "png") setPreviewURL("");
    });

  const setPreview = (blob: Blob): void => {
    setPreviewURL((current) => {
      if (current !== "") URL.revokeObjectURL(current);
      return URL.createObjectURL(blob);
    });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Icon</CardTitle>
        <CardDescription>
          PNG and PIXA are independent owner-managed slots. Each upload is
          limited to 2 MiB.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {previewURL !== "" ? (
          <img
            alt={`${id} icon`}
            className="size-20 rounded-lg border object-contain"
            src={previewURL}
          />
        ) : (
          <ImageIcon className="size-12 text-muted-foreground" />
        )}
        {error !== "" ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : null}
        {(["png", "pixa"] as const).map((format) => {
          const objectName = icon?.[format];
          return (
            <div
              className="flex flex-wrap items-center gap-2 border-t pt-3"
              key={format}
            >
              <span className="w-12 text-sm font-medium uppercase">
                {format}
              </span>
              <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
                {objectName ?? "Not set"}
              </span>
              <Button
                asChild
                disabled={busy !== ""}
                size="sm"
                variant="outline"
              >
                <label>
                  <Upload className="size-4" /> Upload
                  <input
                    accept={
                      format === "png"
                        ? "image/png"
                        : ".pixa,application/octet-stream"
                    }
                    className="hidden"
                    onChange={(event) => {
                      const file = event.target.files?.[0];
                      event.target.value = "";
                      if (file) void upload(format, file);
                    }}
                    type="file"
                  />
                </label>
              </Button>
              <Button
                disabled={busy !== "" || objectName == null}
                onClick={() => void download(format)}
                size="sm"
                type="button"
                variant="outline"
              >
                <Download className="size-4" />{" "}
                {format === "png" ? "Preview" : "Download"}
              </Button>
              <Button
                disabled={busy !== "" || objectName == null}
                onClick={() => void remove(format)}
                size="sm"
                type="button"
                variant="outline"
              >
                <Trash2 className="size-4" /> Delete
              </Button>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}
