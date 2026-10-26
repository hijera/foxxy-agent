package llm

import (
	"fmt"
	"strings"
)

// pictureTypes are the picture types the Anthropic Messages API and the
// OpenAI Responses API take as an image.
var pictureTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// What an attached part is to a provider that takes only pictureTypes.
const (
	attachedPicture    = iota // sent as a picture
	attachedText              // written out as a labelled text block
	attachedUnsendable        // a picture it cannot take, named instead
)

// IsPicture reports whether an attached part reaches a provider as a picture
// rather than as text: an image of any type but SVG, which is text, whether
// the part is a data URL or the file it is kept as, or an https address. A
// provider that takes fewer types names the others instead of sending them,
// so a picture never travels as text.
func IsPicture(ip ImagePart) bool {
	if ip.DataURL == "" {
		return isPictureType(ip.MIMEType)
	}
	if !strings.HasPrefix(ip.DataURL, "data:") {
		return strings.HasPrefix(ip.DataURL, "https://")
	}
	mime, _, _ := parseDataURL(ip.DataURL)
	return isPictureType(mime)
}

// isPictureType reports whether a media type is a picture, SVG not counted.
func isPictureType(mime string) bool {
	return strings.HasPrefix(mime, "image/") && mime != "image/svg+xml"
}

// pictureHasSendableSource reports whether the data URL of a picture names a
// source a provider can fetch or decode: an https address, or a data URL whose
// payload is base64. A data URL with any other payload is not a picture any
// API takes, whatever its media type says.
func pictureHasSendableSource(dataURL string) bool {
	if !strings.HasPrefix(dataURL, "data:") {
		return strings.HasPrefix(dataURL, "https://")
	}
	_, isBase64, _ := parseDataURL(dataURL)
	return isBase64
}

// parseDataURL splits a data URL (RFC 2397) into its media type, lower case
// and without parameters, whether its payload is base64 - the flag is the
// last parameter before the comma, spelled in any case - and the payload.
// Anything that is not a data URL has no type and is its own payload.
func parseDataURL(dataURL string) (mime string, isBase64 bool, payload string) {
	rest, ok := strings.CutPrefix(dataURL, "data:")
	if !ok {
		return "", false, dataURL
	}
	header, payload, found := strings.Cut(rest, ",")
	if !found {
		return "", false, dataURL
	}
	params := strings.Split(header, ";")
	mime = strings.ToLower(strings.TrimSpace(params[0]))
	isBase64 = len(params) > 1 && strings.EqualFold(strings.TrimSpace(params[len(params)-1]), "base64")
	return mime, isBase64, payload
}

// sortAttachment sorts a part for a provider that takes only pictureTypes: a
// base64 data URL of one of them, or an https address, is a picture (with its
// type and base64 payload for a data URL); a data URL of anything else but a
// picture is text to write out, an SVG included, since it is text; any other
// picture - another type, a data URL that is not base64 - cannot be sent.
func sortAttachment(ip ImagePart) (kind int, mime, payload string) {
	if !strings.HasPrefix(ip.DataURL, "data:") {
		if strings.HasPrefix(ip.DataURL, "https://") {
			return attachedPicture, "", ""
		}
		return attachedUnsendable, "", ""
	}
	mime, isBase64, payload := parseDataURL(ip.DataURL)
	if !isPictureType(mime) {
		return attachedText, mime, ""
	}
	if !isBase64 || !pictureTypes[mime] {
		return attachedUnsendable, mime, ""
	}
	return attachedPicture, mime, payload
}

// AttachmentText is attachmentText for the part itself: the text an attached
// part that does not go out as a picture adds to its message. A tool result,
// which can carry no ImagePart at all (the field is the user-message one and
// every provider ignores it elsewhere), writes this into its own content, so
// a file a call brought in reaches the model the way a prompt's text files
// do instead of being dropped.
func AttachmentText(ip ImagePart) string {
	if !IsPicture(ip) {
		return attachmentText(ip, attachedText, "")
	}
	kind, mime, _ := sortAttachment(ip)
	return attachmentText(ip, kind, mime)
}

// attachmentText is the text a part that is not sent as a picture adds to its
// message: the file written out, or a line naming a picture the provider
// cannot take, so the model is never left to guess what came with a prompt.
func attachmentText(ip ImagePart, kind int, mime string) string {
	label := ip.Name
	if label == "" {
		label = "file"
	}
	if kind == attachedUnsendable {
		if mime == "" {
			mime = "picture"
		}
		return fmt.Sprintf("\n\n[File: %s: a %s this provider cannot be sent as a picture]", label, mime)
	}
	return fmt.Sprintf("\n\n[File: %s]\n%s", label, decodeDataURL(ip.DataURL))
}
