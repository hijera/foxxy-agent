import { permissionOptionLabel } from "../chat/permissionOptionLabel";
import { useT } from "../i18n/I18nProvider";

export type DesktopPermissionNotification = {
  kind: "permission";
  id: string;
  sessionId: string;
  toolCallId: string;
  /** Matches the inline transcript prompt id so both dismiss together. */
  itemId: string;
  title: string;
  detail: string;
  options: Array<{ optionId: string; name: string }>;
};

export type DesktopNotification = DesktopPermissionNotification;

/**
 * Bottom-right toast stack for the desktop shell. Mirrors permission prompts
 * with the same action buttons, alongside a notification chime (played by the
 * caller). Rendered only when {@link isDesktopShell} is true; plan-ready and
 * finished-turn alerts go through the host bridge instead.
 */
export function DesktopNotifications(props: {
  notifications: DesktopNotification[];
  onPermissionChoose: (
    n: DesktopPermissionNotification,
    optionId: string,
    label: string,
  ) => void;
  onDismiss: (id: string) => void;
}) {
  const { t } = useT();
  if (props.notifications.length === 0) {
    return null;
  }

  return (
    <div
      className="desktop-toast-stack"
      role="region"
      aria-label={t("desktopNotify.regionLabel")}
      data-testid="desktop-toast-stack"
    >
      {props.notifications.map((n) => (
        <div
          key={n.id}
          className={`desktop-toast desktop-toast--${n.kind}`}
          role="alert"
          data-testid={`desktop-toast-${n.kind}`}
        >
          <button
            type="button"
            className="desktop-toast-close"
            aria-label={t("desktopNotify.dismiss")}
            title={t("desktopNotify.dismiss")}
            data-testid={`desktop-toast-close-${n.id}`}
            onClick={() => props.onDismiss(n.id)}
          >
            ×
          </button>
          <div className="desktop-toast-title">{n.title}</div>
          {n.detail ? (
            <pre className="desktop-toast-detail">{n.detail}</pre>
          ) : null}
          <div className="desktop-toast-actions">
            {n.options.map((opt) => {
              const isReject = opt.optionId === "reject";
              // Same translation as the inline card: the names are the backend's English.
              const label = permissionOptionLabel(opt);
              return (
                <button
                  key={opt.optionId}
                  type="button"
                  className={
                    isReject
                      ? "permission-prompt-btn permission-prompt-btn--reject"
                      : "permission-prompt-btn permission-prompt-btn--allow"
                  }
                  data-testid={`desktop-toast-opt-${opt.optionId}`}
                  onClick={() => props.onPermissionChoose(n, opt.optionId, label)}
                >
                  {label}
                </button>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
