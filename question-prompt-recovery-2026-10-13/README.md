# Question prompt recovery

Captured from the running HTTP UI in headless Chromium with IntelliJ embed mode, Dark theme, 390px and 1280px widths. The local scripted model asks the same four-choice question in both builds. No real provider or user conversation is used.

- `before/missing-*.png`: an HTTP proxy removes only the named `question` SSE frame; the static tool offer remains without answer controls.
- `after/missing-*.png`: the same lost frame is recovered through session activity and all four choices appear.
- `before/rejected-*.png`: an injected HTTP 500 incorrectly resolves the card.
- `after/rejected-*.png`: the selected option and Continue button remain, with an inline error; the subsequent real answer succeeds.
- `after/answered-*.png`: a successful answer produces the normal question/answer record.
- `before/inactive-*.png` and `after/inactive-*.png`: an activity snapshot explicitly reports idle for an unfinished question. The new UI explains inactivity and hides dead answer controls.

The initial missing-event regression, HTTP 404/500 submission regressions, and inactive-runtime regression failed before the change. Real HTTP checks pass after the change at both widths. Normal immediate question delivery remains covered on first POST, subsequent POST, and relay streams.
