import { t } from "../i18n/i18n";
import type { FoxxyCodePermissionOption } from "./permissionTypes";

export function defaultPermissionOptions(): FoxxyCodePermissionOption[] {
  return [
    { optionId: "allow", name: t("prompts.allow"), kind: "allow_once" },
    {
      optionId: "allow_always",
      name: t("prompts.allowAlways"),
      kind: "allow_always",
    },
    { optionId: "reject", name: t("prompts.reject"), kind: "reject_once" },
  ];
}

/** @deprecated use defaultPermissionOptions() for locale-aware labels. */
export const DEFAULT_PERMISSION_OPTIONS = defaultPermissionOptions();
