//go:build windows

package platform

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 윈도우 폴더 선택 창: 파일 올리기에 쓰는 것과 같은 탐색기 창(IFileOpenDialog, 폴더 고르기 모드).
// 예전에는 PowerShell 의 트리만 있는 옛 창(FolderBrowserDialog)을 트레이 프로세스가 띄워서 설정 창 뒤에 숨었다.

var (
	procCoCreateInstance  = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	procSHCreateItem      = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
	user32                = windows.NewLazySystemDLL("user32.dll")
	procEnumThreadWindows = user32.NewProc("EnumThreadWindows")
	procSetWindowPos      = user32.NewProc("SetWindowPos")
	procSetForeground     = user32.NewProc("SetForegroundWindow")
	procGetWindowLong     = user32.NewProc("GetWindowLongW")

	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidIShellItem       = windows.GUID{Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE, Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

// FOS_NOCHANGEDIR | FOS_PICKFOLDERS | FOS_FORCEFILESYSTEM | FOS_PATHMUSTEXIST
const folderDialogOptions = 0x8 | 0x20 | 0x40 | 0x800

const (
	gwlExStyle  = ^uintptr(19) // GWL_EXSTYLE (-20)
	wsExTopmost = 0x8
)

// PickFolderIn 은 owner 창에 붙은(모달) 폴더 선택 창을 띄운다. 떠 있는 동안 owner 창은 잠기고 포커스는 선택 창에 있다.
// owner 창을 만든 UI 스레드(COM STA)에서 불러야 한다. 취소하면 빈 문자열.
func PickFolderIn(owner uintptr, initial, prompt string) (string, error) {
	return showFolderDialog(owner, pickStart(initial), prompt)
}

// pickFolderWindows 는 붙을 창 없이 폴더 선택 창을 띄운다 (설정 화면을 브라우저로 열었을 때 트레이 프로세스가 부른다).
// 백그라운드 프로세스의 창은 윈도우가 앞으로 못 나오게 해서 브라우저 뒤에 숨는다 → 뜨자마자 맨 위로 올린다.
func pickFolderWindows(start, prompt string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil || err == syscall.Errno(1) { // S_OK, S_FALSE
		defer windows.CoUninitialize()
	}
	done := make(chan struct{})
	defer close(done)
	go raiseDialog(windows.GetCurrentThreadId(), done)
	return showFolderDialog(0, start, prompt)
}

var (
	raiseMu    sync.Mutex
	raiseFound windows.HWND
	// 콜백은 프로세스마다 만들 수 있는 수가 정해져 있어 하나만 만든다
	raiseCallback = syscall.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		cls := make([]uint16, 16)
		if n, _ := windows.GetClassName(hwnd, &cls[0], int32(len(cls))); windows.UTF16ToString(cls[:n]) == "#32770" && windows.IsWindowVisible(hwnd) {
			raiseFound = hwnd
			return 0
		}
		return 1
	})
)

// raiseDialog 는 thread 가 띄운 대화 상자(#32770)를 찾아 맨 위로 올리고 앞으로 가져온다.
// 대화 상자는 뜬 직후 스스로 자리를 다시 잡으며 맨 위 속성을 풀기도 해서(타이밍에 따라), 닫힐 때까지 풀리면 다시 올린다.
func raiseDialog(thread uint32, done chan struct{}) {
	var hwnd windows.HWND
	for deadline := time.Now().Add(5 * time.Second); hwnd != 0 || time.Now().Before(deadline); {
		select {
		case <-done:
			return
		case <-time.After(100 * time.Millisecond):
		}
		if hwnd == 0 {
			raiseMu.Lock()
			raiseFound = 0
			procEnumThreadWindows.Call(uintptr(thread), raiseCallback, 0)
			hwnd = raiseFound
			raiseMu.Unlock()
			if hwnd == 0 {
				continue
			}
			procSetForeground.Call(uintptr(hwnd))
		}
		if ex, _, _ := procGetWindowLong.Call(uintptr(hwnd), gwlExStyle); ex&wsExTopmost == 0 {
			const swpNoSize, swpNoMove, hwndTopmost = 0x1, 0x2, ^uintptr(0)
			procSetWindowPos.Call(uintptr(hwnd), hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize)
		}
	}
}

func showFolderDialog(owner uintptr, start, prompt string) (string, error) {
	var dlg *comObject
	if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&dlg))); hr != 0 {
		return "", fmt.Errorf("IFileOpenDialog: 0x%08x", uint32(hr))
	}
	defer dlg.release()
	var opts uint32
	dlg.call(10, uintptr(unsafe.Pointer(&opts)))   // GetOptions
	dlg.call(9, uintptr(opts|folderDialogOptions)) // SetOptions
	if prompt != "" {
		if p, err := windows.UTF16PtrFromString(prompt); err == nil {
			dlg.call(17, uintptr(unsafe.Pointer(p))) // SetTitle
		}
	}
	if start != "" {
		var item *comObject
		if p, err := windows.UTF16PtrFromString(start); err == nil {
			if hr, _, _ := procSHCreateItem.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&item))); hr == 0 {
				dlg.call(12, uintptr(unsafe.Pointer(item))) // SetFolder
				item.release()
			}
		}
	}
	if hr := dlg.call(3, owner); hr != 0 { // Show
		if hr == 0x800704C7 { // HRESULT_FROM_WIN32(ERROR_CANCELLED)
			return "", nil
		}
		return "", fmt.Errorf("IFileOpenDialog.Show: 0x%08x", hr)
	}
	var item *comObject
	if hr := dlg.call(20, uintptr(unsafe.Pointer(&item))); hr != 0 { // GetResult
		return "", fmt.Errorf("IFileOpenDialog.GetResult: 0x%08x", hr)
	}
	defer item.release()
	var name *uint16
	if hr := item.call(5, 0x80058000, uintptr(unsafe.Pointer(&name))); hr != 0 { // GetDisplayName(SIGDN_FILESYSPATH)
		return "", fmt.Errorf("IShellItem.GetDisplayName: 0x%08x", hr)
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(name))
	return windows.UTF16PtrToString(name), nil
}
