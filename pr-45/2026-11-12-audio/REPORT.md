# PR #45: MP4-to-MP3 smoke test (2026-11-12)

Base: main `838aebf4f4e8428a72f58f50c7d318332aa88eb1`.
The demo used an isolated home, a synthetic two-second MP4 (blue video, 440 Hz
sine audio), and real ffmpeg 7.0.2. No external model or user files were used.
The draft was created through `POST /foxxycode/miniapps`; execution was started
from the generated browser form.

## Browser result: failed

1. The form renders a `file` input as a text field, without file selection or
   upload. The editor's input-type selector also has no file option.
2. An absolute input path pauses for command trust. Approving the narrow ffmpeg
   profile succeeds, then the run fails: `path ... escapes its run workspace`.
3. With the relative name `video sample.mp4`, the command starts but ffmpeg reports
   `No such file or directory`: the source is not copied into the fresh workspace.
4. There is no artifact download surface for obtaining an MP3 in the browser.
5. After the first failed run, the old command confirmation remains visible; the
   run history on that screen still displays zero. These are additional UI findings.

Screenshots: `converter-failed.png` and `converter-relative-failed.png`.
Draft: `mp4-to-mp3.json`.

## Engine result: passed with an explicitly supplied fixture

The same document ran through `Runner.RunPortableWithFixture`, using the approved
profile and materializing the real MP4 under the run workspace. Its status was
`succeeded`. `ffprobe` found one `mp3` audio stream, no video stream, duration
2.063673 seconds. The output is 11,146 bytes. SHA-256:
`c04851e1d0fb23cdd7734dc01a69f9fffc9f0ba0e8f817b2c7d5142da55f887c`.

This is an engine smoke test, not a successful browser upload/download flow and
not a test of model-driven session distillation. A usable converter needs file
input ingestion plus artifact delivery; widening path confinement is not a fix.
