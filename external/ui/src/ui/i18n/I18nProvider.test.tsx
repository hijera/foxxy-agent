import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { act, render, screen } from "@testing-library/react";
import React from "react";
import { I18nProvider, useT } from "./I18nProvider";
import { initLocale, setLocale } from "./i18n";
import { CODDY_UI_LANG_COOKIE } from "./localeCookie";

function Probe() {
  const { t } = useT();
  return <span>{t("nav.settings")}</span>;
}

describe("I18nProvider", () => {
  beforeEach(() => {
    document.cookie = `${CODDY_UI_LANG_COOKIE}=; Max-Age=0; Path=/`;
    initLocale("en");
  });

  afterEach(() => {
    document.cookie = `${CODDY_UI_LANG_COOKIE}=; Max-Age=0; Path=/`;
    initLocale("en");
  });

  it("re-renders when setLocale is called", () => {
    render(
      <I18nProvider>
        <Probe />
      </I18nProvider>,
    );
    expect(screen.getByText("Settings")).toBeTruthy();
    act(() => {
      setLocale("ru");
    });
    expect(screen.getByText("Настройки")).toBeTruthy();
  });
});
