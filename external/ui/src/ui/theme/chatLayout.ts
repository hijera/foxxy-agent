import { useSyncExternalStore } from "react";

const storageKey = "foxxycode.chat-layout";
const eventName = "foxxycode:chat-layout";
let fallbackClassic = false;

function readClassicLayout(): boolean {
  try {
    return window.localStorage.getItem(storageKey) === "classic";
  } catch {
    return fallbackClassic;
  }
}

function subscribe(update: () => void): () => void {
  window.addEventListener(eventName, update);
  window.addEventListener("storage", update);
  return () => {
    window.removeEventListener(eventName, update);
    window.removeEventListener("storage", update);
  };
}

export function useClassicChatLayout(): boolean {
  return useSyncExternalStore(subscribe, readClassicLayout, () => false);
}

export function setClassicChatLayout(classic: boolean): void {
  fallbackClassic = classic;
  try {
    window.localStorage.setItem(storageKey, classic ? "classic" : "modern");
  } catch {
    // Some editor webviews block storage; the choice still works for this page.
  }
  window.dispatchEvent(new Event(eventName));
}
