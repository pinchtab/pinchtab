//go:build windows

package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = user32.NewProc("UnregisterHotKey")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procDestroyIcon         = user32.NewProc("DestroyIcon")
)

const (
	wmApp          = 0x8000
	wmTrayIcon     = wmApp + 1
	wmHotkey       = 0x0312
	wmCommand      = 0x0111
	wmLButtonUp    = 0x0202
	wmRButtonUp    = 0x0205

	nimAdd    = 0x00000000
	nimDelete = 0x00000004
	nifIcon   = 0x00000002
	nifTip    = 0x00000004
	nifMessage = 0x00000001
	nifInfo   = 0x00000010

	idOpen = 1001
	idStop = 1002

	mfString = 0x00000000

	tpmBottomAlign = 0x0020
	tpmLeftAlign   = 0x0000

	imageIcon     = 1
	lrLoadFromFile = 0x00000010
	lrDefaultSize  = 0x00000040

	// Hotkey modifiers
	modAlt     = 0x0001
	modControl = 0x0002

	hotkeyID = 1
)

type point struct {
	x, y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type wndClassExW struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

var (
	trayHwnd     uintptr
	trayIcon     uintptr
	dashboardURL string
	stopFunc     func()
	openBrowserFn func(string) error
)

// Run starts the system tray icon and global hotkey. Blocks until quit.
// dashURL is the dashboard URL to open. openBrowser opens a URL in the default browser.
// onStop is called when "Stop PinchTab" is selected.
func Run(dashURL string, openBrowser func(string) error, onStop func()) {
	dashboardURL = dashURL
	stopFunc = onStop
	openBrowserFn = openBrowser

	className := syscall.StringToUTF16Ptr("PinchTabTray")

	wc := wndClassExW{
		wndProc:   syscall.NewCallback(trayWndProc),
		className: className,
	}
	wc.size = uint32(unsafe.Sizeof(wc))

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	)
	trayHwnd = hwnd

	// Load icon from temp file (write embedded ico to temp)
	trayIcon = loadEmbeddedIcon()

	// Add tray icon
	nid := newNotifyIconData(hwnd, trayIcon)
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))

	// Register Ctrl+Alt+A global hotkey
	procRegisterHotKey.Call(hwnd, hotkeyID, modControl|modAlt, 'A')

	// Message loop
	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	// Cleanup
	procUnregisterHotKey.Call(hwnd, hotkeyID)
	nid = newNotifyIconData(hwnd, 0)
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	if trayIcon != 0 {
		procDestroyIcon.Call(trayIcon)
	}
}

// Quit posts a quit message to the tray message loop.
func Quit() {
	if trayHwnd != 0 {
		procPostQuitMessage.Call(0)
	}
}

func trayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmTrayIcon:
		switch lParam {
		case wmLButtonUp:
			openDashboard()
		case wmRButtonUp:
			showContextMenu(hwnd)
		}
		return 0
	case wmHotkey:
		if wParam == hotkeyID {
			openDashboard()
		}
		return 0
	case wmCommand:
		switch wParam {
		case idOpen:
			openDashboard()
		case idStop:
			if stopFunc != nil {
				go stopFunc()
			}
			Quit()
		}
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

func openDashboard() {
	if dashboardURL != "" && openBrowserFn != nil {
		_ = openBrowserFn(dashboardURL)
	}
}

func showContextMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}

	openLabel := syscall.StringToUTF16Ptr("Open Dashboard")
	stopLabel := syscall.StringToUTF16Ptr("Stop PinchTab")

	procAppendMenuW.Call(menu, mfString, idOpen, uintptr(unsafe.Pointer(openLabel)))
	procAppendMenuW.Call(menu, mfString, idStop, uintptr(unsafe.Pointer(stopLabel)))

	procSetForegroundWindow.Call(hwnd)

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procTrackPopupMenu.Call(menu, tpmBottomAlign|tpmLeftAlign, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
	procDestroyMenu.Call(menu)
}

func newNotifyIconData(hwnd, icon uintptr) notifyIconDataW {
	nid := notifyIconDataW{
		hWnd:             hwnd,
		uID:              1,
		uFlags:           nifIcon | nifTip | nifMessage,
		uCallbackMessage: wmTrayIcon,
		hIcon:            icon,
	}
	nid.cbSize = uint32(unsafe.Sizeof(nid))

	tip := syscall.StringToUTF16("PinchTab")
	copy(nid.szTip[:], tip)

	return nid
}

func loadEmbeddedIcon() uintptr {
	// Write embedded icon to a temp file and load it
	tmpDir := os.TempDir()
	icoPath := filepath.Join(tmpDir, "pinchtab-tray.ico")
	if err := os.WriteFile(icoPath, IconData, 0644); err != nil {
		return 0
	}

	pathPtr := syscall.StringToUTF16Ptr(icoPath)
	icon, _, _ := procLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		imageIcon,
		32, 32,
		lrLoadFromFile|lrDefaultSize,
	)
	if icon == 0 {
		// Try 16x16
		icon, _, _ = procLoadImageW.Call(
			0,
			uintptr(unsafe.Pointer(pathPtr)),
			imageIcon,
			16, 16,
			lrLoadFromFile,
		)
	}
	return icon
}

// IsAvailable returns true on Windows.
func IsAvailable() bool {
	return true
}

// DashboardURL returns the configured dashboard URL.
func DashboardURL() string {
	return dashboardURL
}

func init() {
	// Ensure the tip string is valid
	_ = fmt.Sprintf("PinchTab tray initialized")
}
