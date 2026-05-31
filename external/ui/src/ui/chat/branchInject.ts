import type { TranscriptItem } from "./types";

export type BranchPointData = {
  userMessageIndex: number;
  currentIndex: number;
  total: number;
  sessions: Array<{ sessionId: string; preview?: string }>;
};

export function injectBranchNavItems(
  items: TranscriptItem[],
  branchPoints: BranchPointData[],
): TranscriptItem[] {
  if (!branchPoints.length) return items;
  const byUserIdx = new Map<number, BranchPointData>();
  for (const bp of branchPoints) {
    byUserIdx.set(bp.userMessageIndex, bp);
  }
  const result: TranscriptItem[] = [];
  let userMsgCount = 0;
  for (const item of items) {
    result.push(item);
    if (item.type === "user_message") {
      const bp = byUserIdx.get(userMsgCount);
      if (bp) {
        result.push({
          id: `branch-nav-${bp.userMessageIndex}`,
          type: "branch_nav",
          userMessageIndex: bp.userMessageIndex,
          currentIndex: bp.currentIndex,
          total: bp.total,
          sessions: bp.sessions,
        });
      }
      userMsgCount++;
    }
  }
  return result;
}
