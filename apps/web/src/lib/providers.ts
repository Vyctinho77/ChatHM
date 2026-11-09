const LABELS: Record<string, string> = { claude: "Claude", chatgpt: "ChatGPT", grok: "Grok", gemini: "Gemini" };

export const providerLabel = (provider?: string): string => LABELS[provider ?? ""] ?? "Assistente de IA";
