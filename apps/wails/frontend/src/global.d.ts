declare module "*.css";

interface Window {
  runtime?: {
    EventsOn?(name: string, callback: (...args: any[]) => void): () => void;
  };
}

declare namespace JSX {
  type Element = import("react").JSX.Element;
}
