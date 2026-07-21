export type ThinkingProviderData = {
  thinking_level_param?: string;
  thinking_levels?: string[];
  thinking_param?: string;
};

export function hasThinkingToggle(providerData: ThinkingProviderData | undefined): boolean {
  const param = providerData?.thinking_param ?? providerData?.thinking_level_param;
  return (
    param === "enable_thinking" ||
    param === "thinking.type" ||
    providerData?.thinking_levels?.some(isDisabledThinkingLevel) === true
  );
}

export function isDisabledThinkingLevel(level: string): boolean {
  return ["disabled", "disable", "off", "false", "0", "none", "no"].includes(level.trim().toLowerCase());
}
