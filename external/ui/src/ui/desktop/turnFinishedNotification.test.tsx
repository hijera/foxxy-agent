import { act, renderHook, cleanup } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { useSessionTurnActivity } from "../chat/useSessionTurnActivity";

afterEach(cleanup);

test("finished alert fires once even when a silent activity refresh ends the turn", () => {
  const finished = vi.fn();
  const { result } = renderHook(() =>
    useSessionTurnActivity({
      sessionId: "",
      connected: true,
      postPending: () => false,
      onQueueRead: () => () => {},
      onReconcile: () => {},
      onTurnFinished: finished,
    }),
  );
  act(() => {
    result.current.observe("s", false);
  });
  expect(finished).not.toHaveBeenCalled();
  act(() => {
    result.current.observe("s", true);
    result.current.observe("s", false);
    result.current.observe("s", false);
  });
  expect(finished).toHaveBeenCalledExactlyOnceWith("s");
});
