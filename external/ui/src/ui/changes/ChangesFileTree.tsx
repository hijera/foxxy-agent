import { useMemo, useState } from "react";
import { useT } from "../i18n/I18nProvider";
import { buildFileTree } from "./fileTree";
import type { FileTreeNode } from "./fileTree";
import type { ChangedFile } from "./types";

/**
 * The changed files as a tree, for the window's "show files" panel.
 *
 * Clicking a file scrolls the diff list to it rather than filtering: the point
 * of the panel is orientation inside one long document, not a second view of
 * the same data.
 */
export function ChangesFileTree(props: {
  files: ChangedFile[];
  onPick: (path: string) => void;
}) {
  const { t } = useT();
  const [filter, setFilter] = useState("");

  const statuses = useMemo(() => {
    const byPath = new Map<string, string>();
    for (const f of props.files) {
      byPath.set(f.path, f.status);
    }
    return byPath;
  }, [props.files]);

  const nodes = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    const paths = props.files
      .map((f) => f.path)
      .filter((p) => needle === "" || p.toLowerCase().includes(needle));
    return buildFileTree(paths);
  }, [props.files, filter]);

  return (
    <div className="dv-tree" data-testid="dv-tree">
      <input
        type="text"
        className="dv-tree-filter"
        value={filter}
        placeholder={t("changes.viewer.filterFiles")}
        aria-label={t("changes.viewer.filterFiles")}
        data-testid="dv-tree-filter"
        onChange={(e) => setFilter(e.target.value)}
      />
      <div className="dv-tree-rows">
        {nodes.length === 0 ? (
          <div className="dv-note">{t("changes.viewer.noMatches")}</div>
        ) : (
          nodes.map((node) => (
            <TreeRow
              key={node.path}
              node={node}
              depth={0}
              statuses={statuses}
              onPick={props.onPick}
            />
          ))
        )}
      </div>
    </div>
  );
}

function TreeRow(props: {
  node: FileTreeNode;
  depth: number;
  statuses: Map<string, string>;
  onPick: (path: string) => void;
}) {
  const [open, setOpen] = useState(true);
  const node = props.node;
  const indent = { paddingLeft: `${8 + props.depth * 12}px` };

  if (node.kind === "file") {
    const status = props.statuses.get(node.path) || "modified";
    return (
      <button
        type="button"
        className="dv-tree-row dv-tree-row--file"
        style={indent}
        title={node.path}
        data-testid={`dv-tree-file-${node.path}`}
        onClick={() => props.onPick(node.path)}
      >
        <span
          className={"dv-file-badge dv-file-badge--" + status}
          aria-hidden="true"
        />
        <span className="dv-tree-label">{node.label}</span>
      </button>
    );
  }

  return (
    <>
      <button
        type="button"
        className="dv-tree-row dv-tree-row--dir"
        style={indent}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="dv-file-chevron" aria-hidden="true">
          {open ? "\u25BE" : "\u25B8"}
        </span>
        <span className="dv-tree-label">{node.label}</span>
      </button>
      {open
        ? node.children.map((child) => (
            <TreeRow
              key={child.path}
              node={child}
              depth={props.depth + 1}
              statuses={props.statuses}
              onPick={props.onPick}
            />
          ))
        : null}
    </>
  );
}
