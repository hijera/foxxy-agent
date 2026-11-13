import { useRef, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { deleteMiniAppUpload, uploadMiniAppFile } from "./api";
import type { MiniAppDocument, MiniAppInput } from "./types";

type SelectedFile = { upload_id: string; name: string; size_bytes: number };

function FileField(props: {
  app: MiniAppDocument;
  input: MiniAppInput;
  value: unknown;
  onChange: (value: unknown) => void;
  onBusy: (busy: boolean) => void;
}) {
  const { t } = useT();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const generation = useRef(0);
  const selected = (
    Array.isArray(props.value)
      ? props.value
      : props.value && typeof props.value === "object"
        ? [props.value]
        : []
  ) as SelectedFile[];
  const select = async (files: File[]) => {
    const mine = ++generation.current;
    const maximum =
      props.input.type === "files"
        ? Math.min(props.input.validation?.max_files ?? 16, 16)
        : 1;
    if (files.length > maximum) {
      setError(t("miniapps.files.tooMany", { count: String(maximum) }));
      return;
    }
    if (files.length === 0) return;
    setBusy(true);
    props.onBusy(true);
    setError(null);
    const next: SelectedFile[] = [];
    try {
      for (const file of files) {
        const result = await uploadMiniAppFile(props.app, props.input.id, file);
        if (!result.ok) {
          setError(result.message);
          for (const upload of next)
            await deleteMiniAppUpload(props.app.id, upload.upload_id);
          return;
        }
        next.push({
          upload_id: result.data.id,
          name: result.data.name,
          size_bytes: result.data.size_bytes,
        });
      }
      if (mine !== generation.current) return;
      props.onChange(props.input.type === "files" ? next : next[0]);
      for (const previous of selected)
        await deleteMiniAppUpload(props.app.id, previous.upload_id);
    } finally {
      setBusy(false);
      props.onBusy(false);
    }
  };
  return (
    <div
      className="miniapps-file-field"
      onDragOver={(e) => e.preventDefault()}
      onDrop={(e) => {
        e.preventDefault();
        if (!busy) void select(Array.from(e.dataTransfer.files));
      }}
    >
      <label>
        <span>
          {props.input.title}
          {props.input.required ? <em aria-hidden="true"> *</em> : null}
        </span>
        <input
          type="file"
          aria-label={props.input.title}
          disabled={busy}
          multiple={props.input.type === "files"}
          accept={[
            ...(props.input.validation?.extensions ?? []),
            ...(props.input.validation?.media_types ?? []),
          ].join(",")}
          onChange={(e) => {
            void select(Array.from(e.target.files ?? []));
            e.target.value = "";
          }}
        />
      </label>
      <small>
        {busy ? t("miniapps.files.uploading") : t("miniapps.files.drop")}
      </small>
      {props.input.description ? (
        <small>{props.input.description}</small>
      ) : null}
      {selected.map((file) => (
        <div className="miniapps-file-selection" key={file.upload_id}>
          <span>
            {file.name} · {Math.ceil(file.size_bytes / 1024)} KB
          </span>
          <button
            type="button"
            disabled={busy}
            aria-label={t("miniapps.files.remove", { name: file.name })}
            onClick={() => {
              props.onChange(
                props.input.type === "files"
                  ? selected.filter((item) => item.upload_id !== file.upload_id)
                  : "",
              );
              void deleteMiniAppUpload(props.app.id, file.upload_id);
            }}
          >
            ×
          </button>
        </div>
      ))}
      {error ? (
        <p className="miniapps-error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

export function MiniAppForm(props: {
  app: MiniAppDocument;
  values: Record<string, unknown>;
  onChange: (id: string, value: unknown) => void;
  onBusy: (busy: boolean) => void;
}) {
  const { t } = useT();
  const inputs = [...(props.app.inputs ?? [])].sort(
    (a, b) => (a.ui?.order ?? 0) - (b.ui?.order ?? 0),
  );
  const groups = Array.from(
    new Set(inputs.map((input) => input.ui?.group ?? "")),
  );
  const [busyFields, setBusyFields] = useState<Set<string>>(new Set());
  const busyRef = useRef(busyFields);
  const field = (input: MiniAppInput) => {
    const value = props.values[input.id];
    const change = (next: unknown) => props.onChange(input.id, next);
    if (input.type === "file" || input.type === "files")
      return (
        <FileField
          key={input.id}
          app={props.app}
          input={input}
          value={value}
          onChange={change}
          onBusy={(busy) => {
            const next = new Set(busyRef.current);
            if (busy) next.add(input.id);
            else next.delete(input.id);
            busyRef.current = next;
            setBusyFields(next);
            props.onBusy(next.size > 0);
          }}
        />
      );
    const text = value === null || value === undefined ? "" : String(value);
    const control = input.ui?.control;
    return (
      <label key={input.id}>
        <span>
          {input.title}
          {input.required ? <em aria-hidden="true"> *</em> : null}
        </span>
        {input.type === "boolean" ? (
          <span className="miniapps-checkbox">
            <input
              aria-label={input.title}
              type="checkbox"
              checked={value === true}
              onChange={(e) => change(e.target.checked)}
            />
            <span>{t("miniapps.enabled")}</span>
          </span>
        ) : input.validation?.enum ? (
          <select
            aria-label={input.title}
            value={text}
            onChange={(e) => {
              const selected = input.validation?.enum?.find(
                (option) => String(option) === e.target.value,
              );
              change(selected ?? "");
            }}
          >
            <option value="">{t("miniapps.chooseValue")}</option>
            {input.validation.enum.map((option) => (
              <option key={String(option)} value={String(option)}>
                {String(option)}
              </option>
            ))}
          </select>
        ) : control === "textarea" || input.type === "text" ? (
          <textarea
            aria-label={input.title}
            value={text}
            placeholder={input.ui?.placeholder}
            onChange={(e) => change(e.target.value)}
          />
        ) : (
          <input
            aria-label={input.title}
            type={
              input.type === "secret"
                ? "password"
                : input.type === "date"
                  ? "date"
                  : input.type === "datetime"
                    ? "datetime-local"
                    : input.type === "number" || input.type === "integer"
                      ? "number"
                      : "text"
            }
            value={text}
            min={input.validation?.minimum}
            max={input.validation?.maximum}
            step={input.type === "integer" ? 1 : undefined}
            placeholder={input.ui?.placeholder}
            onChange={(e) =>
              change(
                input.type === "integer" || input.type === "number"
                  ? e.target.value === ""
                    ? ""
                    : Number(e.target.value)
                  : e.target.value,
              )
            }
          />
        )}
        {input.description ? <small>{input.description}</small> : null}
      </label>
    );
  };
  return (
    <div
      className={`miniapps-run-form${props.app.display?.layout === "stacked" ? " miniapps-run-form--stacked" : ""}`}
    >
      {inputs.length === 0 ? (
        <p className="miniapps-muted">{t("miniapps.noInputs")}</p>
      ) : null}
      {groups.map((group) =>
        group ? (
          <fieldset className="miniapps-input-group" key={group}>
            <legend>{group}</legend>
            {inputs
              .filter((input) => (input.ui?.group ?? "") === group)
              .map(field)}
          </fieldset>
        ) : (
          inputs.filter((input) => !input.ui?.group).map(field)
        ),
      )}
    </div>
  );
}
