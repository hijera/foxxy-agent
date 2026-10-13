//go:build desktop && windows && amd64

package desktop

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"runtime"
	"testing"

	syswinrt "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/winrt"
)

// Opt-in playback verifies the system notification event and silent custom clips.
func TestNativeCustomSoundPlayback(t *testing.T) {
	if os.Getenv("FOXXYCODE_TEST_NATIVE_AUDIO") != "1" {
		t.Skip("interactive audio device test")
	}
	data := make([]byte, 44+3200)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 3200)
	p := &nativeSoundPlayer{}
	defer p.close()
	// Zero-length spectral data in complete MPEG-1 Layer III frames encodes silence.
	mp3 := make([]byte, 417*15)
	for i := 0; i < len(mp3); i += 417 {
		copy(mp3[i:], []byte{0xff, 0xfb, 0x90, 0xc0})
	}
	for _, audio := range []struct {
		name string
		data []byte
	}{{"Windows default", nil}, {"WAV", data}, {"MP3", mp3}} {
		t.Run(audio.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			if err := syswinrt.RoInitialize(syswinrt.RO_INIT_MULTITHREADED); err != nil {
				t.Fatal(err)
			}
			defer syswinrt.RoUninitialize()
			if err := p.play(base64.StdEncoding.EncodeToString(audio.data)); err != nil {
				t.Fatal(err)
			}
			if !p.playing {
				t.Fatal("native sound did not start")
			}
			p.stop()
			if p.playing {
				t.Fatal("native sound did not stop")
			}
		})
	}
}
