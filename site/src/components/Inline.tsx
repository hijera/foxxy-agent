import { Fragment } from "react";
import { parseInline } from "../../scripts/lib/changelog.mjs";
import type { Block, Inline } from "../app/data";

/** Renders inline tokens; links to other sites open in a new tab. */
export function InlineTokens({ tokens }: { tokens: Inline[] }) {
  return (
    <>
      {tokens.map((token, i) => {
        switch (token.type) {
          case "code":
            return <code key={i}>{token.text}</code>;
          case "strong":
            return <strong key={i}>{token.text}</strong>;
          case "link":
            return (
              <a key={i} href={token.href} target="_blank" rel="noopener noreferrer">
                {token.text}
              </a>
            );
          default:
            return <Fragment key={i}>{token.text}</Fragment>;
        }
      })}
    </>
  );
}

/** Message text with the same light markup as the changelogs: `code`, **strong**, [link](https://…). */
export function Text({ children }: { children: string }) {
  return <InlineTokens tokens={parseInline(children)} />;
}

export function Blocks({ blocks }: { blocks: Block[] }) {
  return (
    <>
      {blocks.map((block, i) =>
        block.type === "p" ? (
          <p key={i}>
            <InlineTokens tokens={block.inlines} />
          </p>
        ) : (
          <ul key={i}>
            {block.items.map((item, j) => (
              <li key={j}>
                <InlineTokens tokens={item} />
              </li>
            ))}
          </ul>
        ),
      )}
    </>
  );
}
