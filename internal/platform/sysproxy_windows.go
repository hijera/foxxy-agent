//go:build windows

package platform

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The system proxy is read and evaluated through WinHTTP, the same service
// Windows itself uses: WinHttpGetIEProxyConfigForCurrentUser returns what the
// Proxy settings page holds, and WinHttpGetProxyForUrl runs the PAC script (or
// the WPAD discovery) for a URL. Go has neither: net/http reads only the
// environment.

var (
	winhttp                                   = windows.NewLazySystemDLL("winhttp.dll")
	procWinHttpGetIEProxyConfigForCurrentUser = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
	procWinHttpOpen                           = winhttp.NewProc("WinHttpOpen")
	procWinHttpSetTimeouts                    = winhttp.NewProc("WinHttpSetTimeouts")
	procWinHttpGetProxyForUrl                 = winhttp.NewProc("WinHttpGetProxyForUrl")
	procGlobalFree                            = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")
)

const (
	winhttpAccessTypeNoProxy    = 1
	winhttpAccessTypeNamedProxy = 3

	winhttpAutoProxyAutoDetect = 0x1
	winhttpAutoProxyConfigURL  = 0x2

	winhttpAutoDetectTypeDHCP = 0x1
	winhttpAutoDetectTypeDNSA = 0x2

	// A PAC download or a WPAD lookup that hangs must not hang every request
	// behind it; past this the lookup fails and the request falls back.
	pacTimeoutMS = 10000
)

// WINHTTP_CURRENT_USER_IE_PROXY_CONFIG
type ieProxyConfig struct {
	autoDetect    int32
	autoConfigURL *uint16
	proxy         *uint16
	proxyBypass   *uint16
}

// WINHTTP_AUTOPROXY_OPTIONS
type autoProxyOptions struct {
	flags                 uint32
	autoDetectFlags       uint32
	autoConfigURL         *uint16
	reserved              uintptr
	reservedDword         uint32
	autoLogonIfChallenged int32
}

// WINHTTP_PROXY_INFO
type proxyInfo struct {
	accessType  uint32
	proxy       *uint16
	proxyBypass *uint16
}

// takeString copies a WinHTTP-allocated string and frees it.
func takeString(p *uint16) string {
	if p == nil {
		return ""
	}
	s := windows.UTF16PtrToString(p)
	_, _, _ = procGlobalFree.Call(uintptr(unsafe.Pointer(p)))
	return s
}

func readSystemProxyConfigOS() (SystemProxyConfig, error) {
	if err := procWinHttpGetIEProxyConfigForCurrentUser.Find(); err != nil {
		return SystemProxyConfig{}, err
	}
	var c ieProxyConfig
	r, _, err := procWinHttpGetIEProxyConfigForCurrentUser.Call(uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		return SystemProxyConfig{}, fmt.Errorf("WinHttpGetIEProxyConfigForCurrentUser: %w", err)
	}
	return SystemProxyConfig{
		AutoDetect:    c.autoDetect != 0,
		AutoConfigURL: takeString(c.autoConfigURL),
		Proxy:         takeString(c.proxy),
		Bypass:        takeString(c.proxyBypass),
	}, nil
}

var pacSession = sync.OnceValues(func() (uintptr, error) {
	agent, _ := windows.UTF16PtrFromString("FoxxyCode")
	h, _, err := procWinHttpOpen.Call(uintptr(unsafe.Pointer(agent)), winhttpAccessTypeNoProxy, 0, 0, 0)
	if h == 0 {
		return 0, fmt.Errorf("WinHttpOpen: %w", err)
	}
	_, _, _ = procWinHttpSetTimeouts.Call(h, pacTimeoutMS, pacTimeoutMS, pacTimeoutMS, pacTimeoutMS)
	return h, nil
})

func pacLookupOS(target string, cfg SystemProxyConfig) (string, error) {
	if err := procWinHttpGetProxyForUrl.Find(); err != nil {
		return "", err
	}
	session, err := pacSession()
	if err != nil {
		return "", err
	}
	wurl, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return "", err
	}
	opts := autoProxyOptions{autoLogonIfChallenged: 1}
	if cfg.AutoConfigURL != "" {
		pac, err := windows.UTF16PtrFromString(cfg.AutoConfigURL)
		if err != nil {
			return "", err
		}
		opts.flags = winhttpAutoProxyConfigURL
		opts.autoConfigURL = pac
	} else {
		opts.flags = winhttpAutoProxyAutoDetect
		opts.autoDetectFlags = winhttpAutoDetectTypeDHCP | winhttpAutoDetectTypeDNSA
	}
	var info proxyInfo
	r, _, callErr := procWinHttpGetProxyForUrl.Call(session,
		uintptr(unsafe.Pointer(wurl)), uintptr(unsafe.Pointer(&opts)), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return "", fmt.Errorf("WinHttpGetProxyForUrl: %w", callErr)
	}
	list := takeString(info.proxy)
	_ = takeString(info.proxyBypass)
	if info.accessType != winhttpAccessTypeNamedProxy {
		return "", nil
	}
	return list, nil
}
