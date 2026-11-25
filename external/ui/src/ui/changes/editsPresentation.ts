import { useSyncExternalStore } from "react";

export type EditsPresentation = "workspace" | "card";

const COOKIE = "foxxycode_edits_presentation";
const CHANGE_EVENT = "foxxycode:edits-presentation";
const MAX_AGE_SECONDS = 365 * 24 * 60 * 60;

const defaultPresentation = (): EditsPresentation => "workspace";

function readEditsPresentation(): EditsPresentation {
  if (typeof document === "undefined") return defaultPresentation();
  const card = document.cookie.split(";").some((part) => part.trim() === COOKIE + "=card");
  return card ? "card" : defaultPresentation();
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener(CHANGE_EVENT, onChange);
  return () => window.removeEventListener(CHANGE_EVENT, onChange);
}

export function useEditsPresentation(): EditsPresentation {
  return useSyncExternalStore(subscribe, readEditsPresentation, defaultPresentation);
}

export function setEditsPresentation(presentation: EditsPresentation): void {
  const secure = window.location.protocol === "https:" ? "; Secure" : "";
  document.cookie =
    COOKIE + "=" + presentation + "; Path=/; Max-Age=" + MAX_AGE_SECONDS + "; SameSite=Lax" + secure;
  window.dispatchEvent(new Event(CHANGE_EVENT));
}
