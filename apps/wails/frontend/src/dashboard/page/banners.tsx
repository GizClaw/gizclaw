import * as React from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";

interface ErrorBannerProps extends React.HTMLAttributes<HTMLDivElement> {
  message: string;
}

const ErrorBanner = React.forwardRef<HTMLDivElement, ErrorBannerProps>(
  ({ message, ...props }, ref) => (
    <Alert ref={ref} variant="destructive" {...props}>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  ),
);
ErrorBanner.displayName = "ErrorBanner";

interface NoticeBannerProps extends React.HTMLAttributes<HTMLDivElement> {
  message: string;
  tone: "error" | "success";
}

const NoticeBanner = React.forwardRef<HTMLDivElement, NoticeBannerProps>(
  ({ message, tone, ...props }, ref) => {
    if (tone === "error") {
      return <ErrorBanner ref={ref} message={message} {...props} />;
    }
    return (
      <Alert ref={ref} variant="success" {...props}>
        <AlertDescription>{message}</AlertDescription>
      </Alert>
    );
  },
);
NoticeBanner.displayName = "NoticeBanner";

export { ErrorBanner, NoticeBanner };
