package agent

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// A picture the provider would refuse on every request it stays in the history
// is left out of the request, the way an over-budget picture is, and the
// pictures around it still go out.
func TestSelectPicturesLeavesOutAPicturePastThePerPictureLimit(t *testing.T) {
	big := llm.ImagePart{Name: "big.png", MIMEType: "image/png", Size: toolImagesMaxPictureBytes}
	small := llm.ImagePart{Name: "small.png", MIMEType: "image/png", Size: 1024}
	msgs := []llm.Message{
		{ImageParts: []llm.ImagePart{small}},
		{ImageParts: []llm.ImagePart{big}},
	}

	kept, _ := selectPictures(msgs, true, nil)

	if _, ok := kept[pictureKey{1, 0}]; ok {
		t.Fatalf("the picture past the per-picture limit was sent")
	}
	if _, ok := kept[pictureKey{0, 0}]; !ok {
		t.Fatalf("the picture within the limit was dropped with it")
	}
}
