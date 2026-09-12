import { useT } from "../i18n/I18nProvider";
import type { BackgroundTask } from "./types";
import { awaitingPermissionCount } from "./taskStatus";

/**
 * The opener for the background tasks panel, sitting under the last message the
 * way a task line does in Claude Code.
 *
 * It belongs at the bottom of the transcript rather than in the nav rail
 * because the tasks belong to this chat: the thing that started them is right
 * above it.
 */
export function BackgroundTasksChip(props: {
  tasks: BackgroundTask[];
  onOpen: () => void;
}) {
  const { t, tp } = useT();
  const running = props.tasks.filter((task) => task.running).length;
  const total = props.tasks.length;

  // Nothing has ever run in this chat, so there is nothing to open.
  if (total === 0) {
    return null;
  }

  const live = running > 0;
  // A detached subagent waiting for an answer has nowhere else to be noticed:
  // the panel is closed by default and the prompt is not in the transcript, so
  // the chip has to say so.
  const awaiting = awaitingPermissionCount(props.tasks);
  // Plural forms differ per language (Russian has three), so the locale's CLDR
  // rules pick the dictionary entry instead of the component choosing one.
  const label =
    awaiting > 0
      ? tp("tasks.chip.awaiting", awaiting)
      : live
        ? tp("tasks.chip.running", running)
        : tp("tasks.chip.total", total);

  return (
    <div className="bgtask-chip-row">
      <button
        type="button"
        className={[
          "bgtask-chip",
          live ? "is-running" : "",
          awaiting > 0 ? "is-awaiting" : "",
        ]
          .filter(Boolean)
          .join(" ")}
        data-testid="bgtask-chip"
        aria-label={t("tasks.chip.ariaLabel", { label })}
        onClick={props.onOpen}
      >
        <span
          className={`bgtask-dot ${
            awaiting > 0
              ? "bgtask-dot--warning"
              : live
                ? "bgtask-dot--running"
                : "bgtask-dot--muted"
          }`}
          aria-hidden="true"
        />
        <span className="bgtask-chip-text">{label}</span>
      </button>
    </div>
  );
}
