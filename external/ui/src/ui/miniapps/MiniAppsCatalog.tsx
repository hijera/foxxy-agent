import { useMemo, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import type { MiniAppCatalogEntry } from "./types";

export function MiniAppsCatalog(props: {
  apps: MiniAppCatalogEntry[];
  selectedId: string | null;
  loading: boolean;
  error: string | null;
  onSelect: (id: string) => void;
  onCreate: () => void;
  canCreate: boolean;
}) {
  const { t } = useT();
  const [query, setQuery] = useState("");
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return props.apps;
    return props.apps.filter((app) =>
      [app.id, app.name, app.description, ...(app.tags ?? [])]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(needle)),
    );
  }, [props.apps, query]);

  return (
    <section
      className="miniapps-catalog"
      aria-labelledby="miniapps-catalog-heading"
    >
      <div className="miniapps-section-head">
        <div>
          <p className="miniapps-eyebrow">{t("miniapps.eyebrow")}</p>
          <h2 id="miniapps-catalog-heading">{t("miniapps.catalogTitle")}</h2>
        </div>
        {props.canCreate ? (
          <button
            type="button"
            className="miniapps-primary"
            onClick={props.onCreate}
          >
            <span aria-hidden="true">+</span>
            {t("miniapps.create")}
          </button>
        ) : null}
      </div>
      <label className="miniapps-search">
        <span className="sr-only">{t("miniapps.searchLabel")}</span>
        <input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder={t("miniapps.searchPlaceholder")}
          aria-label={t("miniapps.searchLabel")}
        />
      </label>
      {props.loading ? (
        <p className="miniapps-muted">{t("miniapps.loading")}</p>
      ) : null}
      {props.error ? (
        <p className="miniapps-error" role="alert">
          {props.error}
        </p>
      ) : null}
      {!props.loading && !props.error && filtered.length === 0 ? (
        <div className="miniapps-empty">
          <strong>{t("miniapps.emptyTitle")}</strong>
          <p>{t("miniapps.emptyDescription")}</p>
          {props.canCreate ? (
            <button
              type="button"
              className="miniapps-secondary"
              onClick={props.onCreate}
            >
              {t("miniapps.emptyAction")}
            </button>
          ) : null}
        </div>
      ) : null}
      <div className="miniapps-catalog-list" role="list">
        {filtered.map((app) => {
          const selected = app.id === props.selectedId;
          return (
            <button
              type="button"
              role="listitem"
              key={app.id}
              className={`miniapps-catalog-row${selected ? " is-selected" : ""}`}
              onClick={() => props.onSelect(app.id)}
              aria-current={selected ? "true" : undefined}
            >
              <span className="miniapps-catalog-row-main">
                <strong>{app.name || app.id}</strong>
                <span>{app.description || t("miniapps.noDescription")}</span>
              </span>
              <span className="miniapps-catalog-row-meta">
                <span
                  className={`miniapps-state miniapps-state--${app.state || "draft"}`}
                >
                  {app.state === "released"
                    ? t("miniapps.released")
                    : t("miniapps.draft")}
                </span>
                {app.version ? <code>{app.version}</code> : null}
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
