import demos from "../data/demos.json";
import type { Lang, Theme } from "./prefs";

export type DemoSurface = "intellij" | "vscode" | "desktop" | "web";

export interface Shot {
  file: string;
  theme: Theme;
  lang: Lang;
  width: number;
  height: number;
}

export const DEMO_SURFACES: DemoSurface[] = ["intellij", "vscode", "desktop", "web"];

export const shots = demos.shots as Record<DemoSurface, Shot[]>;

/**
 * The screenshot that best fits the visitor: the same theme and language, then the same language
 * in the other theme, then the same theme in the other language, then anything.
 */
export function pickShot(list: Shot[], theme: Theme, lang: Lang): Shot | null {
  return (
    list.find((s) => s.theme === theme && s.lang === lang) ??
    list.find((s) => s.lang === lang) ??
    list.find((s) => s.theme === theme) ??
    list[0] ??
    null
  );
}

export function shotUrl(shot: Shot): string {
  return `${import.meta.env.BASE_URL}screenshots/${shot.file}`;
}
