package agent

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// A picture the provider would refuse on every request it stays in the history
// is left out of the request, the way an over-budget picture is, and the
// pictures around it still go out.
func TestSelectPicturesLeavesOutAPicturePastThePerPictureLimit(t *testing.T) {
	// Size is the raw byte count; its base64 payload is 4/3 of it.
	over := llm.ImagePart{Name: "over.png", MIMEType: "image/png", Size: 3932163}
	small := llm.ImagePart{Name: "small.png", MIMEType: "image/png", Size: 1024}
	msgs := []llm.Message{
		{ImageParts: []llm.ImagePart{small}},
		{ImageParts: []llm.ImagePart{over}},
	}

	kept, _ := selectPictures(msgs, true, nil)

	if _, ok := kept[pictureKey{1, 0}]; ok {
		t.Fatalf("the picture past the per-picture limit was sent")
	}
	if _, ok := kept[pictureKey{0, 0}]; !ok {
		t.Fatalf("the picture within the limit was dropped with it")
	}
}

// A picture at exactly the size read_image accepts (3.75 MiB, which is 5 MiB of
// base64) is within the provider's per-image limit and must go out. The data
// URL prefix is not part of that limit: counting it dropped every such picture.
func TestSelectPicturesKeepsAPictureAtTheReadImageLimit(t *testing.T) {
	const readImageLimit = 3*1024*1024 + 768*1024 // read_image's readImageMaxBytes
	atLimit := llm.ImagePart{Name: "edge.png", MIMEType: "image/png", Size: readImageLimit}
	fromDataURL := llm.ImagePart{Name: "edge.png", MIMEType: "image/png",
		DataURL: "data:image/png;base64," + strings.Repeat("A", toolImagesMaxPictureBytes)}
	msgs := []llm.Message{{ImageParts: []llm.ImagePart{atLimit, fromDataURL}}}

	kept, _ := selectPictures(msgs, true, nil)

	if _, ok := kept[pictureKey{0, 0}]; !ok {
		t.Fatalf("a picture of the read_image size was dropped: %d encoded bytes", pictureEncodedBytes(atLimit))
	}
	if _, ok := kept[pictureKey{0, 1}]; !ok {
		t.Fatalf("a data URL whose payload is at the limit was dropped")
	}
}
