declare module "*.css";

interface Window {
  runtime?: {
    EventsOn?(name: string, callback: (...args: any[]) => void): () => void;
    WindowHide?(): void;
    WindowMinimise?(): void;
    WindowToggleMaximise?(): void;
  };
}

declare namespace JSX {
  type Element = import("react").JSX.Element;
}
