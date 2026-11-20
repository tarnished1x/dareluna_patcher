//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	clientW             = 720
	clientH             = 616
	wmCreate            = 0x0001
	wmDestroy           = 0x0002
	wmPaint             = 0x000F
	wmEraseBkgnd        = 0x0014
	wmCommand           = 0x0111
	wmDrawItem          = 0x002B
	wmCtlColorEdit      = 0x0133
	wmSetFont           = 0x0030
	wmAppUpdate         = 0x8001
	wsVisible           = 0x10000000
	wsChild             = 0x40000000
	wsTabStop           = 0x00010000
	wsBorder            = 0x00800000
	wsOverlapped        = 0x00000000
	wsCaption           = 0x00C00000
	wsSysMenu           = 0x00080000
	wsMinimize          = 0x00020000
	wsExClientEdge      = 0x00000200
	esAutoHScroll       = 0x0080
	bsOwnerDraw         = 0x0000000B
	swShow              = 5
	colorWindow         = 5
	idOriginal          = 1001
	idDestination       = 1002
	idBrowseOriginal    = 1101
	idBrowseDestination = 1102
	idInstall           = 1103
	odSelected          = 0x0001
	odDisabled          = 0x0004
	dtCenter            = 0x0001
	dtVCenter           = 0x0004
	dtSingleLine        = 0x0020
	dtLeft              = 0x0000
	transparent         = 1
	psSolid             = 0
	cwUseDefault        = ^uintptr(0x7fffffff)
	mbOK                = 0x0
	mbIconInfo          = 0x40
	mbIconError         = 0x10
	biReturnOnlyFSDirs  = 0x0001
	biNewDialogStyle    = 0x0040
	biNoNewFolderButton = 0x0200
	iccStandardClasses  = 0x00004000
)

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }
type msg struct {
	Hwnd           uintptr
	Message        uint32
	Padding        uint32
	Wparam, Lparam uintptr
	Time           uint32
	Pt             point
	LPrivate       uint32
}
type wndClassEx struct {
	CbSize                             uint32
	Style                              uint32
	WndProc                            uintptr
	CbClsExtra                         int32
	CbWndExtra                         int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type paintStruct struct {
	Hdc                uintptr
	Erase              int32
	RcPaint            rect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type drawItemStruct struct {
	CtlType, CtlID, ItemID, ItemAction, ItemState uint32
	HwndItem, Hdc                                 uintptr
	RcItem                                        rect
	ItemData                                      uintptr
}
type browseInfo struct {
	Owner, Root        uintptr
	DisplayName, Title *uint16
	Flags              uint32
	Pad                uint32
	Callback, Param    uintptr
	Image              int32
	Pad2               uint32
}
type initCommonControls struct {
	Size  uint32
	Flags uint32
}
type uiEvent struct {
	message  string
	percent  int
	finished bool
	report   installResult
	err      error
}

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	gdi32               = syscall.NewLazyDLL("gdi32.dll")
	shell32             = syscall.NewLazyDLL("shell32.dll")
	ole32               = syscall.NewLazyDLL("ole32.dll")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	comctl32            = syscall.NewLazyDLL("comctl32.dll")
	registerClass       = user32.NewProc("RegisterClassExW")
	createWindow        = user32.NewProc("CreateWindowExW")
	defWindowProc       = user32.NewProc("DefWindowProcW")
	showWindow          = user32.NewProc("ShowWindow")
	updateWindow        = user32.NewProc("UpdateWindow")
	messageBox          = user32.NewProc("MessageBoxW")
	getMessage          = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessage     = user32.NewProc("DispatchMessageW")
	postQuit            = user32.NewProc("PostQuitMessage")
	postMessage         = user32.NewProc("PostMessageW")
	sendMessage         = user32.NewProc("SendMessageW")
	enableWindow        = user32.NewProc("EnableWindow")
	getWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	getWindowText       = user32.NewProc("GetWindowTextW")
	setWindowText       = user32.NewProc("SetWindowTextW")
	invalidateRect      = user32.NewProc("InvalidateRect")
	adjustWindowRect    = user32.NewProc("AdjustWindowRect")
	getSystemMetrics    = user32.NewProc("GetSystemMetrics")
	loadCursor          = user32.NewProc("LoadCursorW")
	beginPaint          = user32.NewProc("BeginPaint")
	endPaint            = user32.NewProc("EndPaint")
	fillRect            = user32.NewProc("FillRect")
	drawTextProc        = user32.NewProc("DrawTextW")
	setBkMode           = gdi32.NewProc("SetBkMode")
	setBkColor          = gdi32.NewProc("SetBkColor")
	setTextColor        = gdi32.NewProc("SetTextColor")
	createSolidBrush    = gdi32.NewProc("CreateSolidBrush")
	selectObject        = gdi32.NewProc("SelectObject")
	deleteObject        = gdi32.NewProc("DeleteObject")
	createPen           = gdi32.NewProc("CreatePen")
	roundRect           = gdi32.NewProc("RoundRect")
	polygon             = gdi32.NewProc("Polygon")
	ellipse             = gdi32.NewProc("Ellipse")
	createFont          = gdi32.NewProc("CreateFontW")
	getModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	shBrowse            = shell32.NewProc("SHBrowseForFolderW")
	shGetPath           = shell32.NewProc("SHGetPathFromIDListW")
	coTaskFree          = ole32.NewProc("CoTaskMemFree")
	coInitialize        = ole32.NewProc("CoInitializeEx")
	coUninitialize      = ole32.NewProc("CoUninitialize")
	initControls        = comctl32.NewProc("InitCommonControlsEx")

	hwndMain, hwndOriginal, hwndDestination                  uintptr
	hwndBrowseOriginal, hwndBrowseDestination, hwndInstall   uintptr
	fontSmall, fontMedium, fontHeading, fontButton, fontIcon uintptr
	brushWhite, brushCanvas                                  uintptr
	activeInstall                                            bool
	progressPercent                                          int
	statusLine                                               = ""
	events                                                   = make(chan uiEvent, 24)
	wndProcCallback                                          uintptr
)

func rgb(r, g, b byte) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

var (
	pinkBg      = rgb(255, 245, 249)
	pinkHeader  = rgb(255, 223, 236)
	pinkPanel   = rgb(255, 253, 254)
	pinkBorder  = rgb(245, 193, 216)
	pinkSoft    = rgb(255, 228, 240)
	pinkAccent  = rgb(231, 106, 153)
	pinkPressed = rgb(207, 75, 127)
	pinkText    = rgb(116, 52, 78)
	pinkMuted   = rgb(150, 99, 120)
	white       = rgb(255, 255, 255)
	off         = rgb(212, 192, 200)
)

func utf16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func makeFont(height int32, weight int32, face string) uintptr {
	h, _, _ := createFont.Call(uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(utf16(face))))
	return h
}
func window(class, title string, style uintptr, ex uintptr, x, y, w, h int32, parent, id, instance uintptr) uintptr {
	hWnd, _, _ := createWindow.Call(ex, uintptr(unsafe.Pointer(utf16(class))), uintptr(unsafe.Pointer(utf16(title))), style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, id, instance, 0)
	return hWnd
}
func textFromEdit(handle uintptr) string {
	n, _, _ := getWindowTextLength.Call(handle)
	if n == 0 || n > 32768 {
		return ""
	}
	buffer := make([]uint16, n+1)
	getWindowText.Call(handle, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return strings.TrimSpace(syscall.UTF16ToString(buffer))
}
func textToEdit(handle uintptr, s string) {
	setWindowText.Call(handle, uintptr(unsafe.Pointer(utf16(s))))
}
func showAlert(title, text string, isError bool) {
	mode := uintptr(mbOK | mbIconInfo)
	if isError {
		mode = mbOK | mbIconError
	}
	messageBox.Call(hwndMain, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16(title))), mode)
}
func chooseFolder(title string) string {
	display := make([]uint16, 260)
	bi := browseInfo{Owner: hwndMain, DisplayName: &display[0], Title: utf16(title), Flags: biReturnOnlyFSDirs | biNewDialogStyle}
	pidl, _, _ := shBrowse.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer coTaskFree.Call(pidl)
	path := make([]uint16, 32768)
	ok, _, _ := shGetPath.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(path)
}
func setEnabled(h uintptr, enabled bool) {
	var flag uintptr
	if enabled {
		flag = 1
	}
	enableWindow.Call(h, flag)
}
func notifyUI(event uiEvent) {
	select {
	case events <- event:
	default:
		return
	}
	postMessage.Call(hwndMain, wmAppUpdate, 0, 0)
}
func beginInstall() {
	if activeInstall {
		return
	}
	src := textFromEdit(hwndOriginal)
	out := textFromEdit(hwndDestination)
	if src == "" || out == "" {
		showAlert("Choose your folders ♡", "Please choose the Original Mod Folder and the Installation Folder first.", true)
		return
	}
	activeInstall = true
	progressPercent = 3
	statusLine = "Preparing your patch... ♡"
	for _, h := range []uintptr{hwndOriginal, hwndDestination, hwndBrowseOriginal, hwndBrowseDestination, hwndInstall} {
		setEnabled(h, false)
	}
	invalidateRect.Call(hwndMain, 0, 1)
	go func() {
		report, err := InstallPatch(src, out, func(msg string, percent int) {
			notifyUI(uiEvent{message: msg, percent: percent})
		})
		notifyUI(uiEvent{finished: true, report: report, err: err})
	}()
}

func windowProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmCreate:
		hwndMain = hwnd
		inst, _, _ := getModuleHandle.Call(0)
		fontSmall = makeFont(-15, 400, "Segoe UI")
		fontMedium = makeFont(-18, 600, "Segoe UI")
		fontHeading = makeFont(-39, 700, "Segoe UI")
		fontButton = makeFont(-19, 700, "Segoe UI")
		fontIcon = makeFont(-29, 600, "Segoe UI Symbol")
		brushWhite, _, _ = createSolidBrush.Call(white)
		brushCanvas, _, _ = createSolidBrush.Call(pinkBg)
		editStyle := uintptr(wsChild | wsVisible | wsTabStop | esAutoHScroll)
		hwndOriginal = window("EDIT", "", editStyle, wsExClientEdge, 45, 235, 480, 37, hwnd, idOriginal, inst)
		hwndDestination = window("EDIT", "", editStyle, wsExClientEdge, 45, 357, 480, 37, hwnd, idDestination, inst)
		buttonStyle := uintptr(wsChild | wsVisible | wsTabStop | bsOwnerDraw)
		hwndBrowseOriginal = window("BUTTON", "Browse", buttonStyle, 0, 544, 234, 131, 40, hwnd, idBrowseOriginal, inst)
		hwndBrowseDestination = window("BUTTON", "Browse", buttonStyle, 0, 544, 356, 131, 40, hwnd, idBrowseDestination, inst)
		hwndInstall = window("BUTTON", "Install", buttonStyle, 0, 199, 440, 322, 63, hwnd, idInstall, inst)
		for _, c := range []uintptr{hwndOriginal, hwndDestination} {
			sendMessage.Call(c, wmSetFont, fontSmall, 1)
		}
		for _, c := range []uintptr{hwndBrowseOriginal, hwndBrowseDestination, hwndInstall} {
			sendMessage.Call(c, wmSetFont, fontButton, 1)
		}
		return 0
	case wmCommand:
		switch int(wParam & 0xFFFF) {
		case idBrowseOriginal:
			selected := chooseFolder("Select folder with the three ORIGINAL mod files (extract ZIP first)")
			if selected != "" {
				textToEdit(hwndOriginal, selected)
			}
		case idBrowseDestination:
			selected := chooseFolder("Select destination folder for the three PATCHED mod files")
			if selected != "" {
				textToEdit(hwndDestination, selected)
			}
		case idInstall:
			beginInstall()
		}
		return 0
	case wmEraseBkgnd:
		return 1
	case wmCtlColorEdit:
		setTextColor.Call(wParam, pinkText)
		setBkColor.Call(wParam, white)
		return brushWhite
	case wmDrawItem:
		if lParam != 0 {
			item := (*drawItemStruct)(unsafe.Pointer(lParam))
			paintButton(item)
			return 1
		}
	case wmPaint:
		paintWindow(hwnd)
		return 0
	case wmAppUpdate:
		for {
			select {
			case event := <-events:
				if event.finished {
					activeInstall = false
					for _, h := range []uintptr{hwndOriginal, hwndDestination, hwndBrowseOriginal, hwndBrowseDestination, hwndInstall} {
						setEnabled(h, true)
					}
					if event.err != nil {
						statusLine = "Oops! Please check the message below ♡"
						progressPercent = 0
						showAlert("Patch installation failed", event.err.Error(), true)
					} else {
						progressPercent = 100
						if event.report.Created == 0 {
							statusLine = "Already installed! You're all set ♡"
						} else {
							statusLine = "Installation complete! Enjoy your mod ♥"
						}
						showAlert("Installation complete ♡", fmt.Sprintf("Your patch is ready!\n\nNew files: %d\nAlready installed: %d\n\nThe original mod files were not changed.", event.report.Created, event.report.Existing), false)
					}
				} else {
					statusLine = event.message
					progressPercent = event.percent
				}
			default:
				invalidateRect.Call(hwnd, 0, 1)
				return 0
			}
		}
	case wmDestroy:
		for _, font := range []uintptr{fontSmall, fontMedium, fontHeading, fontButton, fontIcon} {
			if font != 0 {
				deleteObject.Call(font)
			}
		}
		for _, brush := range []uintptr{brushWhite, brushCanvas} {
			if brush != 0 {
				deleteObject.Call(brush)
			}
		}
		postQuit.Call(0)
		return 0
	}
	result, _, _ := defWindowProc.Call(hwnd, message, wParam, lParam)
	return result
}

func drawFill(hdc uintptr, r rect, brush uintptr) {
	fillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
}
func paintedRound(hdc uintptr, r rect, radius int32, bg, border uintptr) {
	b, _, _ := createSolidBrush.Call(bg)
	p, _, _ := createPen.Call(psSolid, 1, border)
	previousBrush, _, _ := selectObject.Call(hdc, b)
	previousPen, _, _ := selectObject.Call(hdc, p)
	roundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
	selectObject.Call(hdc, previousPen)
	selectObject.Call(hdc, previousBrush)
	deleteObject.Call(p)
	deleteObject.Call(b)
}
func writeText(hdc uintptr, s string, r rect, font, color uintptr, flags uintptr) {
	oldFont, _, _ := selectObject.Call(hdc, font)
	setBkMode.Call(hdc, transparent)
	setTextColor.Call(hdc, color)
	t := syscall.StringToUTF16(s)
	drawTextProc.Call(hdc, uintptr(unsafe.Pointer(&t[0])), uintptr(len(t)-1), uintptr(unsafe.Pointer(&r)), flags)
	selectObject.Call(hdc, oldFont)
}
func drawPoly(hdc uintptr, pts []point, bg uintptr) {
	b, _, _ := createSolidBrush.Call(bg)
	p, _, _ := createPen.Call(psSolid, 1, bg)
	oldB, _, _ := selectObject.Call(hdc, b)
	oldP, _, _ := selectObject.Call(hdc, p)
	polygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
	selectObject.Call(hdc, oldP)
	selectObject.Call(hdc, oldB)
	deleteObject.Call(p)
	deleteObject.Call(b)
}
func drawBow(hdc uintptr, x, y int32, scale int32) {
	// Two ribbon loops, tails, and a knot. Original, hand-drawn bow shapes.
	s := func(a int32) int32 { return a * scale / 10 }
	drawPoly(hdc, []point{{x, y + s(17)}, {x - s(24), y + s(2)}, {x - s(26), y + s(31)}}, pinkAccent)
	drawPoly(hdc, []point{{x, y + s(17)}, {x + s(24), y + s(2)}, {x + s(26), y + s(31)}}, pinkAccent)
	drawPoly(hdc, []point{{x - s(4), y + s(18)}, {x - s(21), y + s(48)}, {x - s(7), y + s(41)}, {x + s(1), y + s(53)}}, pinkAccent)
	drawPoly(hdc, []point{{x + s(4), y + s(18)}, {x + s(21), y + s(48)}, {x + s(7), y + s(41)}, {x - s(1), y + s(53)}}, pinkAccent)
	b, _, _ := createSolidBrush.Call(pinkPressed)
	p, _, _ := createPen.Call(psSolid, 1, pinkPressed)
	oldB, _, _ := selectObject.Call(hdc, b)
	oldP, _, _ := selectObject.Call(hdc, p)
	ellipse.Call(hdc, uintptr(x-s(7)), uintptr(y+s(11)), uintptr(x+s(7)), uintptr(y+s(26)))
	selectObject.Call(hdc, oldP)
	selectObject.Call(hdc, oldB)
	deleteObject.Call(p)
	deleteObject.Call(b)
}
func paintButton(item *drawItemStruct) {
	if item == nil {
		return
	}
	bounds := item.RcItem
	active := item.ItemState&odDisabled == 0
	pressed := item.ItemState&odSelected != 0
	isInstall := int(item.CtlID) == idInstall
	bg := pinkSoft
	fg := pinkText
	border := pinkBorder
	label := "Browse ♡"
	if isInstall {
		bg = pinkAccent
		fg = white
		border = pinkAccent
		label = "♥  INSTALL PATCH  ♥"
	}
	if pressed {
		bg = pinkPressed
		fg = white
	}
	if !active {
		bg = off
		fg = white
		border = off
	}
	paintedRound(item.Hdc, bounds, 22, bg, border)
	writeText(item.Hdc, label, bounds, fontButton, fg, dtCenter|dtVCenter|dtSingleLine)
}
func paintWindow(hwnd uintptr) {
	ps := paintStruct{}
	hdc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	drawFill(hdc, rect{0, 0, clientW, clientH}, brushCanvas)
	bh, _, _ := createSolidBrush.Call(pinkHeader)
	drawFill(hdc, rect{0, 0, clientW, 153}, bh)
	deleteObject.Call(bh)
	drawBow(hdc, 91, 35, 9)
	drawBow(hdc, 623, 35, 9)
	writeText(hdc, "dareluna patcher", rect{145, 51, 575, 106}, fontHeading, pinkText, dtCenter|dtVCenter|dtSingleLine)
	writeText(hdc, "♥", rect{324, 120, 345, 150}, fontIcon, pinkAccent, dtCenter)
	writeText(hdc, "♥", rect{375, 120, 396, 150}, fontIcon, pinkAccent, dtCenter)

	paintedRound(hdc, rect{24, 166, 696, 288}, 24, pinkPanel, pinkBorder)
	paintedRound(hdc, rect{24, 298, 696, 418}, 24, pinkPanel, pinkBorder)
	writeText(hdc, "01   ♡  ORIGINAL MOD FOLDER", rect{45, 180, 620, 220}, fontMedium, pinkText, dtLeft|dtVCenter|dtSingleLine)
	writeText(hdc, "02   ♡  INSTALLATION FOLDER", rect{45, 313, 620, 344}, fontMedium, pinkText, dtLeft|dtVCenter|dtSingleLine)
	writeText(hdc, "Extract the original ZIP first. All three original files must be present.", rect{46, 268, 680, 286}, fontSmall, pinkMuted, dtLeft|dtVCenter|dtSingleLine)
	writeText(hdc, "Your three patched files will be created in this folder.", rect{46, 394, 675, 415}, fontSmall, pinkMuted, dtLeft|dtVCenter|dtSingleLine)

	paintedRound(hdc, rect{43, 517, 677, 535}, 18, white, pinkBorder)
	if progressPercent > 0 {
		width := int32(progressPercent) * 630 / 100
		if width < 18 {
			width = 18
		}
		paintedRound(hdc, rect{46, 520, 46 + width, 532}, 14, pinkAccent, pinkAccent)
	}
	writeText(hdc, statusLine, rect{47, 540, 676, 576}, fontSmall, pinkText, dtCenter|dtVCenter|dtSingleLine)
	writeText(hdc, "Original files are verified and never overwritten  ♡", rect{40, 579, 680, 604}, fontSmall, pinkMuted, dtCenter|dtVCenter|dtSingleLine)
}

func main() {
	// Lock UI message loop to one OS thread to keep COM and Win32 UI state coherent.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	coInitialize.Call(0, 2)
	defer coUninitialize.Call()
	ctl := initCommonControls{Size: uint32(unsafe.Sizeof(initCommonControls{})), Flags: iccStandardClasses}
	initControls.Call(uintptr(unsafe.Pointer(&ctl)))
	instance, _, _ := getModuleHandle.Call(0)
	wndProcCallback = syscall.NewCallback(windowProc)
	className := utf16("DarelunaPatcherWindowV1")
	arrowCursor, _, _ := loadCursor.Call(0, 32512)
	wc := wndClassEx{CbSize: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: wndProcCallback,
		Instance: instance, Cursor: arrowCursor, Background: uintptr(colorWindow + 1), ClassName: className}
	reg, _, err := registerClass.Call(uintptr(unsafe.Pointer(&wc)))
	if reg == 0 {
		panic(fmt.Sprintf("Could not register window: %v", err))
	}
	style := uintptr(wsOverlapped | wsCaption | wsSysMenu | wsMinimize)
	outer := rect{0, 0, clientW, clientH}
	adjustWindowRect.Call(uintptr(unsafe.Pointer(&outer)), style, 0)
	w, h := outer.Right-outer.Left, outer.Bottom-outer.Top
	screenW, _, _ := getSystemMetrics.Call(0)
	screenH, _, _ := getSystemMetrics.Call(1)
	x := (int32(screenW) - w) / 2
	y := (int32(screenH) - h) / 2
	hwnd := window("DarelunaPatcherWindowV1", "dareluna patcher", style, 0, x, y, w, h, 0, 0, instance)
	if hwnd == 0 {
		panic("Could not open patcher window")
	}
	hwndMain = hwnd
	showWindow.Call(hwnd, swShow)
	updateWindow.Call(hwnd)
	var message msg
	for {
		value, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(value) <= 0 {
			break
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
}
