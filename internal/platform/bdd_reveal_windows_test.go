//go:build windows

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/cucumber/godog"
	"golang.org/x/sys/windows"
)

const revealDesktopSourceEnv = "FOXXYCODE_TEST_REVEAL_SOURCE"

// Opt-in: this integration test opens its own unique folder on the desktop.
// It observes visibility through Win32, without injecting input or changing
// any existing Explorer windows. Ordinary CI runs do not need a desktop.
func TestRevealDesktopFeature(t *testing.T) {
	if os.Getenv("FOXXYCODE_TEST_REVEAL_DESKTOP") != "1" {
		t.Skip("set FOXXYCODE_TEST_REVEAL_DESKTOP=1 on an interactive Windows desktop")
	}
	var source, folderName string
	suite := godog.TestSuite{
		Name: "file_reveal_desktop",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a Windows desktop source file with spaces Unicode and a comma$`, func() error {
				folderName = fmt.Sprintf("Foxxy reveal %d", time.Now().UnixNano())
				folder := filepath.Join(t.TempDir(), folderName)
				if err := os.Mkdir(folder, 0700); err != nil {
					return err
				}
				source = filepath.Join(folder, "Отчёт, проверка.txt")
				t.Logf("desktop source: %s", source)
				return os.WriteFile(source, []byte("desktop reveal proof\r\n"), 0600)
			})
			sc.Step(`^a backend without a console reveals the source$`, func() error {
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRevealDesktopHelper$", "-test.v") // #nosec G204 -- re-executes this test binary
				AdaptCommand(cmd)
				cmd.Env = append(os.Environ(), revealDesktopSourceEnv+"="+source)
				cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
				out, err := cmd.CombinedOutput()
				t.Logf("windowless backend: %s", out)
				if err != nil {
					return fmt.Errorf("windowless reveal: %w: %s", err, out)
				}
				return nil
			})
			sc.Step(`^Explorer visibly opens the source folder$`, func() error {
				return waitVisibleExplorerFolder(folderName, 10*time.Second)
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/file_reveal_desktop.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("desktop reveal did not show its source folder")
	}
}

func TestRevealDesktopHelper(t *testing.T) {
	source := os.Getenv(revealDesktopSourceEnv)
	if source == "" {
		t.Skip("child half of the desktop reveal test")
	}
	if hasConsoleWindow() {
		t.Fatal("the reveal helper must reproduce the editor's windowless backend")
	}
	if err := RevealFile(source); err != nil {
		t.Fatal(err)
	}
}

// The editor UI test invokes the HTTP route while this read-only observer waits
// for the unique fixture folder. A 204 response alone cannot satisfy this check.
func TestObserveRevealDesktopFolder(t *testing.T) {
	folder := os.Getenv("FOXXYCODE_TEST_REVEAL_OBSERVE_FOLDER")
	if folder == "" {
		t.Skip("editor integration observer")
	}
	if err := waitVisibleExplorerFolder(filepath.Base(folder), 20*time.Second); err != nil {
		t.Fatal(err)
	}
	t.Logf("visible Explorer source folder: %s", folder)
}

func waitVisibleExplorerFolder(name string, timeout time.Duration) error {
	var visible atomic.Bool
	getText := windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowTextW")
	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		window := windows.HWND(hwnd)
		if !windows.IsWindowVisible(window) {
			return 1
		}
		var class [128]uint16
		if _, err := windows.GetClassName(window, &class[0], int32(len(class))); err != nil || windows.UTF16ToString(class[:]) != "CabinetWClass" {
			return 1
		}
		var title [1024]uint16
		_, _, _ = getText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		if strings.Contains(windows.UTF16ToString(title[:]), name) {
			visible.Store(true)
		}
		return 1
	})
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := windows.EnumWindows(callback, nil); err != nil {
			return err
		}
		if visible.Load() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("no visible Explorer window for source folder %q", name)
}
