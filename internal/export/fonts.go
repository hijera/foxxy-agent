package export

import _ "embed"

// DejaVu Sans and DejaVu Sans Mono are permissively-licensed TrueType fonts
// (see assets/LICENSE) with full Latin, Cyrillic, and common punctuation
// coverage. The session PDF renderer embeds them so the exported transcript
// stays readable for non-Latin (e.g. Russian) content — fpdf's built-in core
// fonts only cover Latin-1 and panic on wider code points.
//
// The sans cuts carry body text; the mono cuts carry code blocks and inline
// code, where column alignment is the whole point. Bold is shipped for both
// because the syntax highlighter renders keywords bold.
//
// The four files total ~2.1 MB; they are only compiled into builds that carry
// the `http` tag, so the default CLI binary is unaffected.

//go:embed assets/DejaVuSans.ttf
var dejavuSansRegular []byte

//go:embed assets/DejaVuSans-Bold.ttf
var dejavuSansBold []byte

//go:embed assets/DejaVuSansMono.ttf
var dejavuMonoRegular []byte

//go:embed assets/DejaVuSansMono-Bold.ttf
var dejavuMonoBold []byte
