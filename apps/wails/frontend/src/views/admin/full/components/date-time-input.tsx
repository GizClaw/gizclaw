import { CalendarIcon, X } from "lucide-react";
import { useMemo } from "react";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/components/ui/utils";

type DateTimeInputProps = {
  disabled?: boolean;
  onChange: (value: string) => void;
  value: string;
};

export function DateTimeInput({
  disabled = false,
  onChange,
  value,
}: DateTimeInputProps): JSX.Element {
  const selectedDate = useMemo(() => dateFromRFC3339(value), [value]);
  const timeValue = useMemo(() => timeInputFromRFC3339(value), [value]);

  return (
    <div className="flex gap-2">
      <Popover>
        <PopoverTrigger asChild>
          <Button
            className={cn(
              "min-w-0 flex-1 justify-start px-3 font-normal",
              value.trim() === "" ? "text-muted-foreground" : undefined,
            )}
            disabled={disabled}
            type="button"
            variant="outline"
          >
            <CalendarIcon className="size-4" />
            <span className="truncate">
              {selectedDate
                ? formatDateTime(selectedDate)
                : "Pick date and time"}
            </span>
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-auto p-0">
          <Calendar
            mode="single"
            onSelect={(date) => {
              if (!date) {
                return;
              }
              onChange(rfc3339FromDateAndTime(date, timeValue || "00:00"));
            }}
            selected={selectedDate}
          />
          <Separator />
          <div className="flex items-center gap-3 p-3">
            <span className="text-sm font-medium text-muted-foreground">
              Time
            </span>
            <Input
              className="w-32"
              onChange={(event) => {
                const nextDate = selectedDate ?? new Date();
                onChange(rfc3339FromDateAndTime(nextDate, event.target.value));
              }}
              step={60}
              type="time"
              value={timeValue}
            />
          </div>
        </PopoverContent>
      </Popover>
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              aria-label="Clear datetime"
              className="shrink-0"
              disabled={disabled || value.trim() === ""}
              onClick={() => onChange("")}
              size="icon"
              type="button"
              variant="outline"
            >
              <X className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Clear datetime</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  );
}

function dateFromRFC3339(value: string): Date | undefined {
  const trimmed = value.trim();
  if (trimmed === "") {
    return undefined;
  }
  const time = Date.parse(trimmed);
  if (Number.isNaN(time)) {
    return undefined;
  }
  return new Date(time);
}

function timeInputFromRFC3339(value: string): string {
  const date = dateFromRFC3339(value);
  if (!date) {
    return "";
  }
  const hour = String(date.getHours()).padStart(2, "0");
  const minute = String(date.getMinutes()).padStart(2, "0");
  return `${hour}:${minute}`;
}

function formatDateTime(date: Date): string {
  return date.toLocaleString(undefined, {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "short",
    year: "numeric",
  });
}

function rfc3339FromDateAndTime(date: Date, timeValue: string): string {
  const [hourValue, minuteValue] = timeValue.split(":");
  const hour = Number(hourValue);
  const minute = Number(minuteValue);
  const nextDate = new Date(date);
  nextDate.setHours(
    Number.isFinite(hour) ? hour : 0,
    Number.isFinite(minute) ? minute : 0,
    0,
    0,
  );
  return nextDate.toISOString().replace(/\.\d{3}Z$/, "Z");
}
