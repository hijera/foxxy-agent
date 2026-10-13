// Standard chime and custom MP3/WAV playback for host notifications and previews.
import {
  getNotificationPreferences,
  onNotificationPreferences,
} from "./notificationConfig";

declare global {
  interface Window {
    foxxycodeHostPlaySound?: (
      data: string,
      whenFocused: boolean,
    ) => Promise<unknown>;
    foxxycodeHostStopSound?: () => Promise<unknown>;
  }
}

let clip: AudioBufferSourceNode | null = null;
let chimeGain: GainNode | null = null;
let playingPreview = false;
let playbackRevision = 0;
let cachedFile = "";
let cachedAudio: Promise<ArrayBuffer> | null = null;

function audioBytes(file: string): Promise<ArrayBuffer> {
  if (!cachedAudio || file !== cachedFile) {
    cachedFile = file;
    cachedAudio = fetch(
      `/foxxycode/notification-sound?v=${encodeURIComponent(file)}`,
    ).then(async (res) => {
      if (!res.ok || res.status === 204) throw new Error("sound unavailable");
      const data = await res.arrayBuffer();
      if (data.byteLength > 5 * 1024 * 1024) throw new Error("sound too large");
      return data;
    });
    void cachedAudio.catch(() => {
      if (cachedFile === file) cachedAudio = null;
    });
  }
  return cachedAudio;
}

export async function notificationSoundData(file: string): Promise<string> {
  const bytes = new Uint8Array(await audioBytes(file));
  let binary = "";
  for (let i = 0; i < bytes.length; i += 32768)
    binary += String.fromCharCode(...bytes.subarray(i, i + 32768));
  return btoa(binary);
}

export function stopNotificationSound(): void {
  playbackRevision++;
  try {
    clip?.stop();
  } catch {
    /* Already stopped. */
  }
  clip = null;
  chimeGain?.disconnect();
  chimeGain = null;
  const stopped = window.foxxycodeHostStopSound?.();
  if (stopped) void stopped.catch(() => {});
}

function automaticSoundAllowed(): boolean {
  const prefs = getNotificationPreferences();
  return (
    prefs.enabled &&
    prefs.sound &&
    (prefs.whenFocused ||
      document.visibilityState !== "visible" ||
      !document.hasFocus())
  );
}

async function playSelectedSound(
  file: string,
  preview: boolean,
): Promise<void> {
  stopNotificationSound();
  const revision = playbackRevision;
  playingPreview = preview;
  if (window.foxxycodeHostPlaySound) {
    const data = file ? await notificationSoundData(file) : "";
    if (revision !== playbackRevision || (!preview && !automaticSoundAllowed()))
      return;
    await window.foxxycodeHostPlaySound(
      data,
      preview || getNotificationPreferences().whenFocused,
    );
    return;
  }
  const c = ensureContext();
  if (!c) {
    if (file) throw new Error("audio unavailable");
    return;
  }
  if (!file) {
    playStandardSound(revision, preview);
    return;
  }
  const buffer = await c.decodeAudioData((await audioBytes(file)).slice(0));
  if (c.state === "suspended") await c.resume();
  if (revision !== playbackRevision || (!preview && !automaticSoundAllowed()))
    return;
  const source = c.createBufferSource();
  source.buffer = buffer;
  source.connect(c.destination);
  clip = source;
  source.start();
}

export function previewNotificationSound(file: string): Promise<void> {
  return playSelectedSound(file, true);
}

export function playNotificationSound(): void {
  if (!automaticSoundAllowed()) return;
  const playback = playSelectedSound(
    getNotificationPreferences().soundFile,
    false,
  );
  const revision = playbackRevision;
  void playback.catch(() => {
    if (revision === playbackRevision && automaticSoundAllowed())
      playStandardSound(revision, false);
  });
}

let previousFile = getNotificationPreferences().soundFile;
onNotificationPreferences(() => {
  const prefs = getNotificationPreferences();
  if (!prefs.enabled || !prefs.sound || prefs.soundFile !== previousFile)
    stopNotificationSound();
  previousFile = prefs.soundFile;
});
window.addEventListener("focus", () => {
  if (!playingPreview && !getNotificationPreferences().whenFocused)
    stopNotificationSound();
});
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
function playStandardSound(revision: number, preview: boolean): void {
  const c = ensureContext();
  if (!c) return;

  const start = () => {
    if (revision !== playbackRevision || (!preview && !automaticSoundAllowed()))
      return;
    try {
      const now = c.currentTime;
      const master = c.createGain();
      chimeGain = master;
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
