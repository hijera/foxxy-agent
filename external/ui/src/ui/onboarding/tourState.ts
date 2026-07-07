// Persistence for the one-shot guided tour ("seen" flag). The tour plays once
// per machine after the onboarding form is dismissed in the desktop shell;
// "Restart onboarding" in Settings resets it so it can play again.

const STORAGE_KEY = "foxxycode.tourSeen";

export function isTourSeen(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}

export function markTourSeen(): void {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(STORAGE_KEY, "1");
  } catch {
    // Ignore storage failures (private mode / disabled storage).
  }
}

export function resetTour(): void {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Ignore storage failures.
  }
}
