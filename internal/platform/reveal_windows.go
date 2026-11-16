//go:build windows

package platform

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	revealShell32   = windows.NewLazySystemDLL("shell32.dll")
	revealOle32     = windows.NewLazySystemDLL("ole32.dll")
	parseRevealPath = revealShell32.NewProc("SHParseDisplayName")
	openRevealItem  = revealShell32.NewProc("SHOpenFolderAndSelectItems")
	initRevealCOM   = revealOle32.NewProc("CoInitializeEx")
	uninitRevealCOM = revealOle32.NewProc("CoUninitialize")
	freeRevealPIDL  = revealOle32.NewProc("CoTaskMemFree")
)

func revealFile(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("artifact source path: %w", err)
	}
	// COM initialization, Shell calls and cleanup belong to one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const (
		apartmentThreaded = 0x2
		disableOLE1DDE    = 0x4
		changedMode       = 0x80010106
	)
	result, _, _ := initRevealCOM.Call(0, apartmentThreaded|disableOLE1DDE)
	if uint32(result) == changedMode {
		// Another component already initialized COM on this thread. Keep its
		// apartment and leave its initialization count untouched.
	} else if err := revealHRESULT("initialize Windows Shell", result); err != nil {
		return err
	} else {
		defer func() { _, _, _ = uninitRevealCOM.Call() }()
	}
	var item uintptr
	result, _, _ = parseRevealPath.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&item)), 0, 0)
	runtime.KeepAlive(name)
	if err := revealHRESULT("resolve artifact source", result); err != nil {
		return err
	}
	if item == 0 {
		return errors.New("windows shell returned no artifact source item")
	}
	defer func() { _, _, _ = freeRevealPIDL.Call(item) }()
	// A fully qualified file PIDL with cidl=0 opens its parent and selects the
	// file. No explorer.exe quoting or hidden-window startup flags are involved.
	// https://learn.microsoft.com/windows/win32/api/shlobj_core/nf-shlobj_core-shopenfolderandselectitems
	result, _, _ = openRevealItem.Call(item, 0, 0, 0)
	return revealHRESULT("show artifact source in Explorer", result)
}

// Shell and COM functions return HRESULT directly, not through GetLastError.
func revealHRESULT(operation string, result uintptr) error {
	if int32(uint32(result)) < 0 {
		return fmt.Errorf("%s: HRESULT 0x%08X", operation, uint32(result))
	}
	return nil
}
