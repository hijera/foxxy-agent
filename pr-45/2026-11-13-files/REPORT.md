# File-processing Mini Apps: MP4 → MP3 (2026-11-13)

The earlier smoke test's missing upload/download path is now implemented. The
browser renders file controls, typed settings, field groups and ordering from
the app document; the result includes a player and download action.

## Real-file browser verification

`TestMiniAppsBrowserAudioExtractor` uses the production embedded SPA, HTTP
handlers, persisted jobs, real ffmpeg and ffprobe. It creates synthetic videos
locally and requires no external model or user data.

- Select and upload an MP4 whose filename contains spaces.
- Review and approve the ffmpeg profile; the same run id continues afterward.
- Complete the test, load the MP3 in its player and download it through Chrome.
- Independently confirm a single MP3 audio stream and no video using ffprobe.
- Reload the browser and reopen the persisted player/download.
- Release version 1.0.0, upload a different video and complete a second run.
- Upload a video without audio: the run fails and no old confirmation is shown.
- Open a previous successful result from history.
- Capture Dark at 1280/390 px and Light at 1280 px, checking horizontal overflow.

The HTTP BDD suite also restarts the service and downloads the same file again,
then verifies the draft without reuploading its pinned private source fixture.
Domain tests cover revision/input binding, filename and content constraints,
byte limits, changed output files, symlink rejection and confirmation continuity.

## Screenshots

`audio-before-*.png` are fresh captures of the previous running build.
`audio-editor-*`, `audio-form-*`, `audio-trust-*`, `audio-result-*` and
`audio-no-track-*` show the updated production build.

The complete portable example is `audio-extractor.json`. It needs a locally
installed ffmpeg executable (or the profile's explicitly approved installer).
Uploads are limited to 256 MiB/file, 16 files/input and 512 MiB staged/app.
Staging expires after 24 hours; passing-test source fixtures remain private and
are invalidated when the draft is saved. Declared output files persist per run.
