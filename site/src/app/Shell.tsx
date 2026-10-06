import { StrictMode, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { PrefsProvider } from "./prefs";
import { Footer, Header, type PageId } from "../components/Header";
import "../styles/tokens.css";
import "../styles/base.css";

export function Page({ page, children }: { page: PageId; children: ReactNode }) {
  return (
    <PrefsProvider page={page}>
      <Header page={page} />
      <main id="main">{children}</main>
      <Footer />
    </PrefsProvider>
  );
}

export function mount(page: PageId, content: ReactNode) {
  const root = document.getElementById("root");
  if (!root) throw new Error("#root is missing");
  createRoot(root).render(
    <StrictMode>
      <Page page={page}>{content}</Page>
    </StrictMode>,
  );
}
