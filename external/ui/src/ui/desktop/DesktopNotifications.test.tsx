import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import {
  DesktopNotifications,
  type DesktopNotification,
} from "./DesktopNotifications";

afterEach(() => cleanup());

const permission: DesktopNotification = {
  kind: "permission",
  id: "perm_tc1",
  sessionId: "s1",
  toolCallId: "tc1",
  itemId: "pp_tc1",
  title: "Run Command",
  detail: "rm -rf build",
  options: [
    { optionId: "allow", name: "Allow" },
    { optionId: "allow_always", name: "Allow always" },
    { optionId: "reject", name: "Reject" },
  ],
};

const plan: DesktopNotification = {
  kind: "plan",
  id: "plan_s1:my-plan",
  sessionId: "s1",
  slug: "my-plan",
  title: "Plan ready",
  body: "Some plan overview",
};

test("renders nothing when there are no notifications", () => {
  const { container } = render(
    <DesktopNotifications
      notifications={[]}
      onPermissionChoose={() => {}}
      onRunPlan={() => {}}
      onDismiss={() => {}}
    />,
  );
  expect(container.firstChild).toBeNull();
});

test("permission toast fires the chosen option", () => {
  const onChoose = vi.fn();
  render(
    <DesktopNotifications
      notifications={[permission]}
      onPermissionChoose={onChoose}
      onRunPlan={() => {}}
      onDismiss={() => {}}
    />,
  );
  expect(screen.getByTestId("desktop-toast-permission")).toBeInTheDocument();
  expect(screen.getByText("rm -rf build")).toBeInTheDocument();
  fireEvent.click(screen.getByTestId("desktop-toast-opt-allow_always"));
  expect(onChoose).toHaveBeenCalledWith(
    permission,
    "allow_always",
    "Allow always",
  );
});

test("plan toast runs and dismisses", () => {
  const onRun = vi.fn();
  const onDismiss = vi.fn();
  render(
    <DesktopNotifications
      notifications={[plan]}
      onPermissionChoose={() => {}}
      onRunPlan={onRun}
      onDismiss={onDismiss}
    />,
  );
  fireEvent.click(screen.getByTestId("desktop-toast-run-plan"));
  expect(onRun).toHaveBeenCalledWith(plan);
  fireEvent.click(screen.getByTestId("desktop-toast-close-plan_s1:my-plan"));
  expect(onDismiss).toHaveBeenCalledWith("plan_s1:my-plan");
});
