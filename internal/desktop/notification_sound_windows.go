//go:build desktop && windows && amd64

package desktop

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/hajimehoshi/go-mp3"
	"github.com/hijera/foxxycode-agent/internal/notificationsound"
)

var winmmDLL = syscall.NewLazyDLL("winmm.dll")

type nativeSoundPlayer struct {
	dir     string
	playing bool
}

// Decode MP3 in process: the legacy MPEG MCI driver is unavailable on some desktops.
func notificationPCM(data []byte) ([]byte, error) {
	d, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	const maxPCM = 128 << 20
	pcm, err := io.ReadAll(io.LimitReader(d, maxPCM+1))
	if err != nil {
		return nil, err
	}
	if len(pcm) == 0 || len(pcm) > maxPCM {
		return nil, fmt.Errorf("notification audio exceeds decoded size limit or is empty")
	}
	wav := make([]byte, 44+len(pcm))
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 2)
	binary.LittleEndian.PutUint32(wav[24:], uint32(d.SampleRate()))
	binary.LittleEndian.PutUint32(wav[28:], uint32(d.SampleRate()*4))
	binary.LittleEndian.PutUint16(wav[32:], 4)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav, nil
}

func mciSoundCommand(command string) error {
	ptr, err := syscall.UTF16PtrFromString(command)
	if err != nil {
		return err
	}
	code, _, _ := winmmDLL.NewProc("mciSendStringW").Call(uintptr(unsafe.Pointer(ptr)), 0, 0, 0)
	if code != 0 {
		buf := make([]uint16, 256)
		_, _, _ = winmmDLL.NewProc("mciGetErrorStringW").Call(code, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		return fmt.Errorf("notification audio %s: MCI error %d: %s", strings.Fields(command)[0], code, syscall.UTF16ToString(buf))
	}
	return nil
}

func (p *nativeSoundPlayer) stop() {
	p.playing = false
	_ = mciSoundCommand("close fxsound")
	_, _, _ = winmmDLL.NewProc("PlaySoundW").Call(0, 0, 0)
}

func (p *nativeSoundPlayer) close() {
	p.stop()
	if p.dir != "" {
		_ = os.RemoveAll(p.dir)
	}
}

// The preview and native notifications share one voice, replacing any previous clip.
func (p *nativeSoundPlayer) play(encoded string) error {
	p.stop()
	if encoded == "" {
		alias, _ := syscall.UTF16PtrFromString("Notification.Default")
		ok, _, _ := winmmDLL.NewProc("PlaySoundW").Call(uintptr(unsafe.Pointer(alias)), 0, 0x210003) // SND_ALIAS | SND_ASYNC | SND_NODEFAULT | SND_SYSTEM
		if ok == 0 {
			return fmt.Errorf("notification audio unavailable")
		}
		p.playing = true
		return nil
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > notificationsound.MaxBytes+2 {
		return notificationsound.ErrTooLarge
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	kind, err := notificationsound.Format(data)
	if err != nil {
		return err
	}
	if kind == "mp3" {
		data, err = notificationPCM(data)
		if err != nil {
			return err
		}
	}
	if p.dir == "" {
		p.dir, err = os.MkdirTemp("", "foxxycode-sound-*")
		if err != nil {
			return err
		}
	}
	path := filepath.Join(p.dir, "sound.wav")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	// Legacy MCI drivers reject some long/Unicode paths even through the wide API.
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	short := make([]uint16, len(path)*2+256)
	n, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetShortPathNameW").Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&short[0])), uintptr(len(short)))
	if n > 0 && n < uintptr(len(short)) {
		path = syscall.UTF16ToString(short[:n])
	}
	if err := mciSoundCommand(`open "` + path + `" type waveaudio alias fxsound`); err != nil {
		return err
	}
	if err := mciSoundCommand("play fxsound from 0"); err != nil {
		p.stop()
		return err
	}
	p.playing = true
	return nil
}

func nativeSoundAllowed() bool {
	var state uint32
	code, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("SHQueryUserNotificationState").Call(uintptr(unsafe.Pointer(&state)))
	return code == 0 && state == 5 // QUNS_ACCEPTS_NOTIFICATIONS
}
