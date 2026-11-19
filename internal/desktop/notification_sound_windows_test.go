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
			prep, err := p.prepare(base64.StdEncoding.EncodeToString(audio.data))
			if err != nil {
				t.Fatal(err)
			}
			if err := p.play(prep); err != nil {
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

// Staging runs without an audio device, so its limits and cleanup are checked here.
func TestNativeSoundPrepare(t *testing.T) {
	p := &nativeSoundPlayer{}
	defer p.close()
	if prep, err := p.prepare(""); err != nil || !prep.system {
		t.Fatalf("empty payload must select the system alias: %v %v", prep, err)
	}
	wav := make([]byte, 44+8)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 8)
	prep, err := p.prepare(base64.StdEncoding.EncodeToString(wav))
	if err != nil {
		t.Fatal(err)
	}
	if prep.system || prep.file == "" {
		t.Fatalf("WAV payload must stage a file: %+v", prep)
	}
	if _, err := os.Stat(prep.file); err != nil {
		t.Fatal(err)
	}
	p.discard(prep)
	if _, err := os.Stat(prep.file); !os.IsNotExist(err) {
		t.Fatalf("discarded clip must be removed: %v", err)
	}
	if _, err := p.prepare(base64.StdEncoding.EncodeToString([]byte("not audio"))); err == nil {
		t.Fatal("non-audio payload must be rejected")
	}
}
