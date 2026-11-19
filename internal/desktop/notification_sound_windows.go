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
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/hajimehoshi/go-mp3"
	"github.com/hijera/foxxycode-agent/internal/notificationsound"
)

var (
	winmmDLL          = syscall.NewLazyDLL("winmm.dll")
	procPlaySound     = winmmDLL.NewProc("PlaySoundW")
	procMCISendString = winmmDLL.NewProc("mciSendStringW")
	procMCIGetError   = winmmDLL.NewProc("mciGetErrorStringW")
	procShortPath     = kernel32DLL.NewProc("GetShortPathNameW")
)

// preparedSound is audio staged by prepare: either the Windows notification
// alias or a temp WAV file MCI can open. Decoding and file IO happen on the
// caller's goroutine so a large MP3 never blocks the notification worker.
type preparedSound struct {
	system bool
	file   string
}

type nativeSoundPlayer struct {
	mu        sync.Mutex
	dir       string
	current   string // staged WAV MCI is playing; removed on stop
	mciOpen   bool
	playing   bool
	dingUntil time.Time // the PlaySound alias has no completion event
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
	code, _, _ := procMCISendString.Call(uintptr(unsafe.Pointer(ptr)), 0, 0, 0)
	if code != 0 {
		buf := make([]uint16, 256)
		_, _, _ = procMCIGetError.Call(code, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		return fmt.Errorf("notification audio %s: MCI error %d: %s", strings.Fields(command)[0], code, syscall.UTF16ToString(buf))
	}
	return nil
}

// mciSoundStatus returns an MCI status string, or "" when the query fails.
func mciSoundStatus(command string) string {
	ptr, err := syscall.UTF16PtrFromString(command)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 128)
	code, _, _ := procMCISendString.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(buf))
}

// mciShortPath works around legacy MCI drivers rejecting long or Unicode paths
// even through the wide API.
func mciShortPath(path string) (string, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	short := make([]uint16, len(path)*2+256)
	n, _, _ := procShortPath.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&short[0])), uintptr(len(short)))
	if n > 0 && n < uintptr(len(short)) {
		return syscall.UTF16ToString(short[:n]), nil
	}
	return path, nil
}

// prepare stages the clip off the worker thread. An empty payload selects the
// Windows notification alias; anything else is decoded, capped and written as a
// unique WAV next to the previous clips.
func (p *nativeSoundPlayer) prepare(encoded string) (preparedSound, error) {
	if encoded == "" {
		return preparedSound{system: true}, nil
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > notificationsound.MaxBytes+2 {
		return preparedSound{}, notificationsound.ErrTooLarge
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return preparedSound{}, err
	}
	kind, err := notificationsound.Format(data)
	if err != nil {
		return preparedSound{}, err
	}
	if kind == "mp3" {
		if data, err = notificationPCM(data); err != nil {
			return preparedSound{}, err
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dir == "" {
		if p.dir, err = os.MkdirTemp("", "foxxycode-sound-*"); err != nil {
			return preparedSound{}, err
		}
	}
	file := filepath.Join(p.dir, fmt.Sprintf("sound-%d.wav", time.Now().UnixNano()))
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return preparedSound{}, err
	}
	return preparedSound{file: file}, nil
}

// discard drops a staged clip the worker decided not to play.
func (p *nativeSoundPlayer) discard(s preparedSound) {
	if s.file == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = os.Remove(s.file)
}

// The preview and native notifications share one voice, replacing any previous clip.
func (p *nativeSoundPlayer) play(s preparedSound) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	if s.system {
		alias, _ := syscall.UTF16PtrFromString("Notification.Default")
		ok, _, _ := procPlaySound.Call(uintptr(unsafe.Pointer(alias)), 0, 0x210003) // SND_ALIAS | SND_ASYNC | SND_NODEFAULT | SND_SYSTEM
		if ok == 0 {
			return fmt.Errorf("notification audio unavailable")
		}
		p.playing = true
		p.dingUntil = time.Now().Add(4 * time.Second)
		return nil
	}
	path, err := mciShortPath(s.file)
	if err != nil {
		return err
	}
	if err := mciSoundCommand(`open "` + path + `" type waveaudio alias fxsound`); err != nil {
		return err
	}
	if err := mciSoundCommand("play fxsound from 0"); err != nil {
		_ = mciSoundCommand("close fxsound")
		return err
	}
	p.mciOpen = true
	p.current = s.file
	p.playing = true
	return nil
}

func (p *nativeSoundPlayer) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

func (p *nativeSoundPlayer) stopLocked() {
	p.playing = false
	p.dingUntil = time.Time{}
	if p.mciOpen {
		_ = mciSoundCommand("close fxsound")
		p.mciOpen = false
	}
	_, _, _ = procPlaySound.Call(0, 0, 0) // cancel an async alias sound
	if p.current != "" {
		_ = os.Remove(p.current)
		p.current = ""
	}
}

// finished reports natural completion so the focus ticker can stop watching:
// MCI reports its mode, while the system alias is bounded by a short deadline.
func (p *nativeSoundPlayer) finished() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.playing {
		return true
	}
	if p.mciOpen {
		return mciSoundStatus("status fxsound mode") != "playing"
	}
	return !time.Now().Before(p.dingUntil)
}

func (p *nativeSoundPlayer) close() {
	p.stop()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dir != "" {
		_ = os.RemoveAll(p.dir)
	}
}

func nativeSoundAllowed() bool {
	var state uint32
	code, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("SHQueryUserNotificationState").Call(uintptr(unsafe.Pointer(&state)))
	return code == 0 && state == 5 // QUNS_ACCEPTS_NOTIFICATIONS
}
