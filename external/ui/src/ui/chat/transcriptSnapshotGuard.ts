export function shouldApplyTranscriptSnapshot(p: {
  requestEpoch: number;
  currentEpoch: number;
  activeComposer: boolean;
  allowWhileActive?: boolean;
}): boolean {
  if (p.requestEpoch !== p.currentEpoch) {
    return false;
  }
  return !p.activeComposer || p.allowWhileActive === true;
}
