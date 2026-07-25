//go:build http && ui && browser

package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

const (
	layoutServerStartupTimeout = 10 * time.Second
	layoutBrowserTimeout       = 20 * time.Second
	layoutProcessStopTimeout   = 3 * time.Second
)

type layoutDevServer struct {
	baseURL string
	cmd     *exec.Cmd
	done    chan struct{}
	log     bytes.Buffer
	waitErr error
	once    sync.Once
}

func TestUILayoutColumnsAlign(t *testing.T) {
	server := startLayoutDevServer(t)
	t.Cleanup(func() {
		server.stop(t)
	})
	var browserLog bytes.Buffer

	browserCtx, browserCancel := context.WithTimeout(
		context.Background(),
		layoutBrowserTimeout,
	)
	defer browserCancel()

	allocatorOptions := append(
		[]chromedp.ExecAllocatorOption{},
		chromedp.DefaultExecAllocatorOptions[:]...,
	)
	allocatorOptions = append(
		allocatorOptions,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-gpu-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.UserDataDir(t.TempDir()),
		chromedp.CombinedOutput(&browserLog),
	)
	if executable := strings.TrimSpace(os.Getenv("FOXXYCODE_UI_BROWSER")); executable != "" {
		allocatorOptions = append(allocatorOptions, chromedp.ExecPath(executable))
	}

	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(
		browserCtx,
		allocatorOptions...,
	)
	defer allocatorCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocatorCtx)
	defer tabCancel()

	for _, viewport := range []struct {
		name   string
		width  int64
		height int64
	}{
		{name: "narrow", width: 390, height: 844},
		{name: "wide", width: 1280, height: 900},
	} {
		t.Run(viewport.name, func(t *testing.T) {
			var result layoutEdges
			if err := chromedp.Run(
				tabCtx,
				chromedp.EmulateViewport(viewport.width, viewport.height),
				chromedp.Navigate(server.baseURL+"/layout-scroll-check.html"),
				chromedp.WaitReady(".chat-header", chromedp.ByQuery),
				chromedp.WaitReady(".messages-inner > :first-child", chromedp.ByQuery),
				chromedp.WaitReady(".composer-card", chromedp.ByQuery),
				chromedp.Evaluate(layoutEdgesExpression, &result),
			); err != nil {
				t.Fatalf(
					"run layout check at %dpx: %v\nbrowser output:\n%s",
					viewport.width,
					err,
					browserLog.String(),
				)
			}
			assertAligned(t, "message", result.Header, result.Message)
			assertAligned(t, "composer", result.Header, result.Composer)
		})
	}
}

type layoutRect struct {
	Left  float64 `json:"left"`
	Right float64 `json:"right"`
}

type layoutEdges struct {
	Header   layoutRect `json:"header"`
	Message  layoutRect `json:"message"`
	Composer layoutRect `json:"composer"`
}

const layoutEdgesExpression = `(() => {
	const rect = (selector) => {
		const element = document.querySelector(selector);
		if (!element) throw new Error("missing layout element: " + selector);
		const bounds = element.getBoundingClientRect();
		return { left: bounds.left, right: bounds.right };
	};
	return {
		header: rect(".chat-header"),
		message: rect(".messages-inner > :first-child"),
		composer: rect(".composer-card"),
	};
})()`

func assertAligned(t *testing.T, name string, want, got layoutRect) {
	t.Helper()
	const tolerance = 1.0
	if math.Abs(want.Left-got.Left) > tolerance ||
		math.Abs(want.Right-got.Right) > tolerance {
		t.Errorf(
			"%s edges do not match header within %.1fpx: header=(%.2f, %.2f), %s=(%.2f, %.2f)",
			name,
			tolerance,
			want.Left,
			want.Right,
			name,
			got.Left,
			got.Right,
		)
	}
}

func startLayoutDevServer(t *testing.T) *layoutDevServer {
	t.Helper()

	node := strings.TrimSpace(os.Getenv("FOXXYCODE_UI_NODE"))
	if node == "" {
		var err error
		node, err = exec.LookPath("node")
		if err != nil {
			t.Fatalf("find node executable: %v", err)
		}
	}

	viteEntry := filepath.Join("node_modules", "vite", "bin", "vite.js")
	if _, err := os.Stat(viteEntry); err != nil {
		t.Fatalf("find Vite entry point %q (run npm ci first): %v", viteEntry, err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Vite port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved Vite port: %v", err)
	}

	server := &layoutDevServer{
		baseURL: "http://127.0.0.1:" + strconv.Itoa(port),
		done:    make(chan struct{}),
	}
	server.cmd = exec.Command(
		node,
		viteEntry,
		"--configLoader",
		"runner",
		"--host",
		"127.0.0.1",
		"--port",
		strconv.Itoa(port),
		"--strictPort",
	)
	server.cmd.Env = append(
		os.Environ(),
		"FOXXYCODE_UI_VITE_CACHE_DIR="+t.TempDir(),
	)
	server.cmd.Stdout = &server.log
	server.cmd.Stderr = &server.log
	if err := server.cmd.Start(); err != nil {
		t.Fatalf("start Vite: %v", err)
	}
	go func() {
		server.waitErr = server.cmd.Wait()
		close(server.done)
	}()

	startupCtx, startupCancel := context.WithTimeout(
		context.Background(),
		layoutServerStartupTimeout,
	)
	defer startupCancel()

	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-server.done:
			t.Fatalf(
				"Vite exited before readiness: %v\n%s",
				server.waitErr,
				server.log.String(),
			)
		case <-startupCtx.Done():
			server.stop(t)
			t.Fatalf(
				"Vite did not become ready within %s\n%s",
				layoutServerStartupTimeout,
				server.log.String(),
			)
		case <-ticker.C:
			if layoutFixtureReady(client, server.baseURL+"/layout-scroll-check.html") {
				return server
			}
		}
	}
}

func layoutFixtureReady(client *http.Client, target string) bool {
	response, err := client.Get(target)
	if err != nil {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	closeErr := response.Body.Close()
	return response.StatusCode == http.StatusOK &&
		err == nil &&
		closeErr == nil &&
		bytes.Contains(body, []byte("data-layout-scroll-check"))
}

func (s *layoutDevServer) stop(t *testing.T) {
	t.Helper()
	s.once.Do(func() {
		select {
		case <-s.done:
			return
		default:
		}
		if err := s.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("stop Vite process: %v", err)
			return
		}
		select {
		case <-s.done:
		case <-time.After(layoutProcessStopTimeout):
			t.Errorf(
				"Vite process did not stop within %s (pid %d)",
				layoutProcessStopTimeout,
				s.cmd.Process.Pid,
			)
		}
	})
}
