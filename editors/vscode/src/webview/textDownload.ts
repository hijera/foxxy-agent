/** Frozen SPA → host request, mirrored in external/ui/src/ui/embedTextDownload.ts. */
export const DOWNLOAD_TEXT_MESSAGE_TYPE = "foxxycode:downloadText";

export interface TextDownload {
  filename: "diagram.mmd" | "diagram.svg" | "image.svg";
  text: string;
}

/** Accept only the text formats and default names the chat's diagram controls offer. */
export function textDownloadFromMessage(value: unknown): TextDownload | null {
  if (!value || typeof value !== "object") return null;
  const message = value as Record<string, unknown>;
  if (message.type !== DOWNLOAD_TEXT_MESSAGE_TYPE || typeof message.text !== "string") return null;
  const filename = message.filename;
  if (filename !== "diagram.mmd" && filename !== "diagram.svg" && filename !== "image.svg") return null;
  return { filename, text: message.text };
}
