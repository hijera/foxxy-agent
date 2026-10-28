# Pictures in download-arrow exports

Real IntelliJ IDEA Community 2022.2.1/JCEF on Windows, dark theme, 1280 px viewport.
The isolated persisted chat contains one uploaded repository icon, one picture returned by read, and a final assistant answer. No live model was used for this export-only reproduction.

- Clicked the actual arrow and HTML/PDF/DOCX/JSON menu items in IntelliJ. The editor file route saved all four documents.
- HTML contains two embedded data images; both decode in Chrome at their original dimensions. DOCX has two media files and passes ZIP CRC. PDF contains both image resources. JSON retains both attachment paths/names.
- Tool arguments and output text are absent. The HTTP BDD uses text extraction to check PDF/DOCX in both browser-download and editor-file routes.
- Before uses the previous test backend against the same persisted chat: only the upload remains. After is the actual HTML file saved by the IntelliJ arrow, opened in Chrome.
- All seven permanent IntelliJ UI tests pass after restoring their standard fixture home. The initial run used the export-specific home, so two hardcoded-home fixture tests could not find their seeded sessions; it was rerun with the correct setup.
- No new FoxxyCode ERROR or TransactionGuard entries during this run. The bundled JetBrains Package Search plugin logged an unrelated DNS warning. Sandbox exited normally in 4.2 seconds and reaped its backend.

Full-suite, lint, documentation and ZIP integrity results are recorded in the PR description.
