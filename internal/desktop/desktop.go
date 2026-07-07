//go:build desktop && windows

package desktop

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/hijera/foxxycode-agent/external/httpserver"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
	"github.com/jchv/go-webview2"
)

// Options configures the desktop launcher.
type Options struct {
	Args            []string
	EnsureHome      func(string) error
	BootstrapConfig func(string) error
	OpenStore       func(string, *config.Config) (*session.FileStore, error)
	NewServerRef    func(**acp.Server, *config.Config, func() *config.Config) acp.UpdateSender
}

// Run starts the embedded HTTP server and opens a WebView2 window.
func Run(opts Options) error {
	if opts.EnsureHome == nil || opts.OpenStore == nil || opts.NewServerRef == nil {
		return fmt.Errorf("desktop: incomplete Options")
	}

	fs := flag.NewFlagSet("desktop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfgPath := fs.String("config", "", "path to config.yaml")
	logLevel := fs.String("log-level", "", "debug|info|warn|error")
	homeDir := fs.String("home", "", "agent state directory (FOXXYCODE_HOME)")
	desktopCWD := fs.String("cwd", "", "default session cwd (FOXXYCODE_CWD)")
	sessionsRoot := fs.String("sessions-dir", "", "sessions root")
	persistedSession := fs.String("session-id", "", "optional session id for new sessions")
	schedulerEnabled := fs.Bool("scheduler-enabled", true, "enable the scheduler daemon (default on in desktop; pass=false to disable)")

	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage of desktop:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(opts.Args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	cli := config.CLIPaths{
		Home:   strings.TrimSpace(*homeDir),
		CWD:    strings.TrimSpace(*desktopCWD),
		Config: strings.TrimSpace(*cfgPath),
	}
	paths, err := config.Resolve(cli)
	if err != nil {
		return err
	}
	if err := opts.EnsureHome(paths.Home); err != nil {
		return err
	}
	if opts.BootstrapConfig != nil {
		if err := opts.BootstrapConfig(paths.Home); err != nil {
			return err
		}
	}

	listenAddr, err := FreeLoopbackPort()
	if err != nil {
		return err
	}

	logFile := filepath.Join(paths.Home, "desktop.log")
	picker := &folderPicker{}
	st, err := httpserver.StartHTTP(httpserver.CommandDeps{
		NewServerRef: opts.NewServerRef,
		EnsureHome:   opts.EnsureHome,
		OpenStore:    opts.OpenStore,
	}, httpserver.StartParams{
		CLI:          cli,
		SessionsRoot: *sessionsRoot,
		SessionID:    *persistedSession,
		ListenAddr:   listenAddr,
		FolderPicker: picker.Pick,
		LoggerOverrides: config.LoggerCLIOverrides{
			Level:  strings.TrimSpace(*logLevel),
			Output: "file",
			File:   logFile,
			Format: "text",
		},
		SchedulerEnabled: *schedulerEnabled,
	})
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- st.ListenAndServe()
	}()

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer readyCancel()
	if err := waitHTTPReady(readyCtx, st.ListenAddr); err != nil {
		_ = st.Shutdown(context.Background())
		return err
	}

	startURL := DesktopStartURL(st.ListenAddr, st.Config.UI.Locale)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "FoxxyCode",
			Width:  1280,
			Height: 800,
			Center: true,
		},
	})
	if w == nil {
		showWebView2InstallHint("FoxxyCode requires Microsoft Edge WebView2 Runtime.\n\nInstall from:\nhttps://developer.microsoft.com/en-us/microsoft-edge/webview2/")
		_ = st.Shutdown(context.Background())
		return fmt.Errorf("desktop: webview2 unavailable")
	}
	defer w.Destroy()
	picker.hwnd.Store(uintptr(w.Window()))
	w.Navigate(startURL)
	w.Run()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = st.Shutdown(shutdownCtx)

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	default:
	}
	return nil
}

func waitHTTPReady(ctx context.Context, listenAddr string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	url := "http://" + listenAddr + "/v1/models"
	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			res, err := client.Do(req)
			if err == nil {
				_ = res.Body.Close()
				if res.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("desktop: http server not ready on %s", listenAddr)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func showWebView2InstallHint(msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")
	title, _ := syscall.UTF16PtrFromString("FoxxyCode")
	text, _ := syscall.UTF16PtrFromString(msg)
	_, _, _ = messageBoxW.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
