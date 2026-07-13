// Notification chime for desktop toasts (permission prompts / plan ready).
//
// Synthesized with the Web Audio API so we ship no binary asset and stay within
// the Chromium 104 baseline (see .claude/rules/ui-spa.md). The AudioContext is
// created lazily and resumed on the first user gesture, because browsers start
// it suspended until the page has been interacted with.

type WebkitWindow = Window & {
  webkitAudioContext?: typeof AudioContext;
};

let ctx: AudioContext | null = null;
let gestureUnlockArmed = false;

function audioContextCtor(): typeof AudioContext | undefined {
  if (typeof window === "undefined") return undefined;
  const w = window as WebkitWindow;
  return window.AudioContext ?? w.webkitAudioContext;
}

function ensureContext(): AudioContext | null {
  if (ctx) return ctx;
  const Ctor = audioContextCtor();
  if (!Ctor) return null;
  try {
    ctx = new Ctor();
  } catch {
    ctx = null;
  }
  return ctx;
}

/**
 * Arm a one-shot listener that resumes the AudioContext on the first pointer /
 * key gesture. By the time a permission prompt appears the user has usually
 * already interacted, but this guarantees the very first chime is not swallowed
 * by the autoplay policy.
 */
export function armNotificationSoundUnlock(): void {
  if (gestureUnlockArmed || typeof window === "undefined") return;
  gestureUnlockArmed = true;
  const unlock = () => {
    const c = ensureContext();
    if (c && c.state === "suspended") {
      void c.resume().catch(() => {});
    }
  };
  window.addEventListener("pointerdown", unlock, { once: true, passive: true });
  window.addEventListener("keydown", unlock, { once: true });
}

/** Play a short two-tone notification chime. No-op when Web Audio is unavailable. */
export function playNotificationSound(): void {
  const c = ensureContext();
  if (!c) return;

  const start = () => {
    try {
      const now = c.currentTime;
      const master = c.createGain();
      master.gain.setValueAtTime(0.0001, now);
      master.connect(c.destination);

      // Two rising notes (E6 -> A6), each a soft sine blip.
      const tones: Array<[number, number]> = [
        [1318.5, now],
        [1760.0, now + 0.13],
      ];
      for (const [freq, at] of tones) {
        const osc = c.createOscillator();
        const g = c.createGain();
        osc.type = "sine";
        osc.frequency.setValueAtTime(freq, at);
        g.gain.setValueAtTime(0.0001, at);
        g.gain.exponentialRampToValueAtTime(0.14, at + 0.015);
        g.gain.exponentialRampToValueAtTime(0.0001, at + 0.12);
        osc.connect(g);
        g.connect(master);
        osc.start(at);
        osc.stop(at + 0.14);
      }
    } catch {
      // ignore audio failures — sound is a non-critical enhancement
    }
  };

  if (c.state === "suspended") {
    void c
      .resume()
      .then(start)
      .catch(() => {});
    return;
  }
  start();
}
