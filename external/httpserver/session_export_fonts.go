//go:build http

package httpserver

import _ "embed"

// DejaVu Sans is a permissively-licensed TrueType font (see
// export_assets/LICENSE) with full Latin, Cyrillic, and common punctuation
// coverage. The session PDF renderer embeds the regular and bold cuts so the
// exported transcript stays readable for non-Latin (e.g. Russian) content —
// fpdf's built-in core fonts only cover Latin-1 and panic on wider code points.
//
// Both cuts are ~700 KB; they are only compiled into builds that carry the
// `http` tag, so the default CLI binary is unaffected.

//go:embed export_assets/DejaVuSans.ttf
var dejavuSansRegular []byte

//go:embed export_assets/DejaVuSans-Bold.ttf
var dejavuSansBold []byte
