import React from "react";
import { afterEach, expect, test } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { NavRail } from "./NavRail";

afterEach(() => cleanup());

test("nav brand new-chat affordance renders a plus icon (compact rail)", () => {
  render(
    <NavRail
      onNewChat={() => {}}
      onOpenHistory={() => {}}
      historyOpen={false}
      onOpenScheduler={() => {}}
      schedulerOpen={false}
      onOpenSettings={() => {}}
      settingsOpen={false}
      canWidenRail={false}
      railLabelsWide={false}
      onToggleRailLabels={() => {}}
    />,
  );

  const home = screen.getByRole("link", { name: "FoxxyCode agent home" });
  expect(home).toBeInTheDocument();
  expect(home.querySelector(".rail-brand-plus")).not.toBeNull();
  expect(home).not.toHaveTextContent("agent");
});

test("nav brand new-chat affordance renders a plus icon (wide header row)", () => {
  render(
    <NavRail
      onNewChat={() => {}}
      onOpenHistory={() => {}}
      historyOpen={false}
      onOpenScheduler={() => {}}
      schedulerOpen={false}
      onOpenSettings={() => {}}
      settingsOpen={false}
      canWidenRail
      railLabelsWide
      onToggleRailLabels={() => {}}
    />,
  );

  const home = screen.getByTestId("nav-home");
  expect(home).toBeInTheDocument();
  expect(home.querySelector(".rail-brand-plus")).not.toBeNull();
  expect(home).not.toHaveTextContent("FoxxyCode");
});

test("nav hides Scheduler when showScheduler is false", () => {
  render(
    <NavRail
      onNewChat={() => {}}
      onOpenHistory={() => {}}
      historyOpen={false}
      showScheduler={false}
      onOpenScheduler={() => {}}
      schedulerOpen={false}
      onOpenSettings={() => {}}
      settingsOpen={false}
      canWidenRail={false}
      railLabelsWide={false}
      onToggleRailLabels={() => {}}
    />,
  );

  expect(screen.queryByTestId("nav-scheduler")).toBeNull();
});

test("in-app nav links expose hash hrefs for new-tab open", () => {
  render(
    <NavRail
      onNewChat={() => {}}
      onOpenHistory={() => {}}
      historyOpen={false}
      onOpenScheduler={() => {}}
      schedulerOpen={false}
      onOpenSettings={() => {}}
      settingsOpen={false}
      canWidenRail={false}
      railLabelsWide={false}
      onToggleRailLabels={() => {}}
    />,
  );
  expect(screen.getByTestId("nav-home")).toHaveAttribute("href", "#/");
  expect(screen.getByTestId("nav-history")).toHaveAttribute("href", "#/history");
  expect(screen.getByTestId("nav-scheduler")).toHaveAttribute("href", "#/scheduler");
  expect(screen.getByTestId("nav-settings")).toHaveAttribute("href", "#/settings");
});
