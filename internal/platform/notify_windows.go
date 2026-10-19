//go:build windows

package platform

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/afy0817/GodseonSync/internal/icon"
)

// 윈도우 알림: WinRT 토스트를 직접 부른다 (PowerShell 을 띄우지 않는다).
// 앱 ID 'GodseonSync' 를 이름·아이콘과 함께 등록해서 알림이 GodseonSync 로 나온다 (예전에는 PowerShell 의 앱 ID 를 빌려
// 'Windows PowerShell' 알림으로 나오고, 누르면 PowerShell 이 열렸다). 알림을 누르면 같이 등록한 godseonsync: 주소로
// `GodseonSync open` 이 실행돼 설정 창이 열린다 (onClick 대신).

const (
	appID     = "GodseonSync"
	aumidKey  = `Software\Classes\AppUserModelId\` + appID
	schemeKey = `Software\Classes\godseonsync`
	toastOpen = "godseonsync:open"
)

var (
	notifyMu       sync.Mutex
	notifyError    func(string)
	notifyReported bool

	combase                    = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize           = combase.NewProc("RoInitialize")
	procRoUninitialize         = combase.NewProc("RoUninitialize")
	procRoActivateInstance     = combase.NewProc("RoActivateInstance")
	procRoGetActivationFactory = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString    = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString    = combase.NewProc("WindowsDeleteString")

	iidToastManagerStatics = windows.GUID{Data1: 0x50AC103F, Data2: 0xD235, Data3: 0x4598, Data4: [8]byte{0xBB, 0xEF, 0x98, 0xFE, 0x4D, 0x1A, 0x3A, 0xD4}}
	iidToastFactory        = windows.GUID{Data1: 0x04124B20, Data2: 0x82C6, Data3: 0x4229, Data4: [8]byte{0xB1, 0x09, 0xFD, 0x9E, 0xD4, 0x66, 0x2B, 0x53}}
	iidXmlDocumentIO       = windows.GUID{Data1: 0x6CD0E74E, Data2: 0xEE65, Data3: 0x4489, Data4: [8]byte{0x9E, 0xBF, 0xCA, 0x43, 0xE8, 0x7B, 0xA6, 0x37}}
	iidXmlDocument         = windows.GUID{Data1: 0xF7F3A506, Data2: 0x1E87, Data3: 0x42D6, Data4: [8]byte{0xBC, 0xFB, 0xB8, 0xC8, 0x09, 0xFA, 0x54, 0x94}}

	errToastDisabled = errors.New("disabled")
)

func setupNotify(onClick func(), onError func(string)) {
	notifyMu.Lock()
	notifyError = onError
	notifyMu.Unlock()
	if SeparateHome() { // 시험용 인스턴스는 실제 앱의 등록(누르면 실행할 파일)을 바꾸지 않는다
		return
	}
	if err := registerNotify(Executable()); err != nil {
		reportNotify(err.Error())
	}
}

// registerNotify 는 알림에 보일 앱 이름·아이콘과, 알림을 누르면 실행할 명령을 이 사용자 레지스트리에 등록한다.
// 지우는 것은 설치 프로그램이 한다 (GodseonSync.iss [Registry]).
func registerNotify(exe string) error {
	if err := os.MkdirAll(AppDir(), 0o755); err != nil {
		return err
	}
	png := filepath.Join(AppDir(), "notify-icon.png")
	if err := os.WriteFile(png, icon.PNG(icon.Render(256, icon.AppStyle(0.02))), 0o644); err != nil {
		return err
	}
	for _, v := range []struct{ key, name, value string }{
		{aumidKey, "DisplayName", "GodseonSync"},
		{aumidKey, "IconUri", png},
		{schemeKey, "", "URL:GodseonSync"},
		{schemeKey, "URL Protocol", ""},
		{schemeKey + `\shell\open\command`, "", `"` + exe + `" open`},
	} {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, v.key, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = k.SetStringValue(v.name, v.value)
		k.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// registered 는 GodseonSync 앱 ID 가 등록돼 있는지다 (시험용 인스턴스는 실제 앱이 등록했을 때만).
func registered() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, aumidKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue("DisplayName")
	return err == nil
}

func reportNotify(msg string) {
	notifyMu.Lock()
	fn, done := notifyError, notifyReported
	notifyReported = true
	notifyMu.Unlock()
	if fn != nil && !done {
		fn(msg)
	}
}

func nativeNotify(title, message string) bool {
	own := registered()
	id := appID
	if !own { // 등록하지 못했으면 PowerShell 의 앱 ID 를 빌린다 (누를 곳은 없음)
		id = psAppID
	}
	err := showToast(id, toastXML(title, message, own))
	if err == nil {
		return true
	}
	reportNotify(err.Error())
	return errors.Is(err, errToastDisabled) // 사용자가 끈 알림은 다른 길로 보내지 않는다
}

// toastXML 은 토스트 내용이다. 우리 앱 ID 면 맨 위에 앱 이름이 나오니 제목이 앱 이름과 같으면 빼고(맥과 같게), 누르면 설정 창을 연다.
func toastXML(title, message string, own bool) string {
	var b strings.Builder
	b.WriteString(`<toast`)
	if own {
		b.WriteString(` activationType="protocol" launch="` + toastOpen + `"`)
	}
	b.WriteString(`><visual><binding template="ToastGeneric">`)
	for _, s := range []string{title, message} {
		if s == "" || (own && s == "GodseonSync") {
			continue
		}
		b.WriteString("<text>")
		_ = xml.EscapeText(&b, []byte(s))
		b.WriteString("</text>")
	}
	b.WriteString(`</binding></visual></toast>`)
	return b.String()
}

// hstring 은 WinRT 문자열을 만든다. 다 쓰면 free 를 부른다.
func hstring(s string) (h uintptr, free func(), err error) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, nil, err
	}
	if hr, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h))); hr != 0 {
		return 0, nil, fmt.Errorf("WindowsCreateString: 0x%08x", uint32(hr))
	}
	return h, func() { procWindowsDeleteString.Call(h) }, nil
}

// factory 는 WinRT 클래스의 정적 메서드(활성화 팩터리) 인터페이스다.
func factory(class string, iid *windows.GUID) (*comObject, error) {
	h, free, err := hstring(class)
	if err != nil {
		return nil, err
	}
	defer free()
	var f *comObject
	if hr, _, _ := procRoGetActivationFactory.Call(h, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&f))); hr != 0 {
		return nil, fmt.Errorf("%s: 0x%08x", class, uint32(hr))
	}
	return f, nil
}

// showToast 는 앱 ID aumid 로 토스트를 띄운다.
func showToast(aumid, content string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr, _, _ := procRoInitialize.Call(1); hr == 0 || hr == 1 { // RO_INIT_MULTITHREADED → S_OK, S_FALSE
		defer procRoUninitialize.Call()
	}

	// XmlDocument 에 내용을 읽힌다
	cls, free, err := hstring("Windows.Data.Xml.Dom.XmlDocument")
	if err != nil {
		return err
	}
	defer free()
	var inst *comObject
	if hr, _, _ := procRoActivateInstance.Call(cls, uintptr(unsafe.Pointer(&inst))); hr != 0 {
		return fmt.Errorf("XmlDocument: 0x%08x", uint32(hr))
	}
	defer inst.release()
	docIO, err := inst.query(&iidXmlDocumentIO)
	if err != nil {
		return err
	}
	defer docIO.release()
	text, freeText, err := hstring(content)
	if err != nil {
		return err
	}
	defer freeText()
	if hr := docIO.call(6, text); hr != 0 { // LoadXml
		return fmt.Errorf("LoadXml: 0x%08x", hr)
	}
	doc, err := inst.query(&iidXmlDocument)
	if err != nil {
		return err
	}
	defer doc.release()

	// ToastNotification 을 만들고 앱 ID 의 알림 보내는 곳으로 보낸다
	tf, err := factory("Windows.UI.Notifications.ToastNotification", &iidToastFactory)
	if err != nil {
		return err
	}
	defer tf.release()
	var toast *comObject
	if hr := tf.call(6, uintptr(unsafe.Pointer(doc)), uintptr(unsafe.Pointer(&toast))); hr != 0 { // CreateToastNotification
		return fmt.Errorf("CreateToastNotification: 0x%08x", hr)
	}
	defer toast.release()
	mgr, err := factory("Windows.UI.Notifications.ToastNotificationManager", &iidToastManagerStatics)
	if err != nil {
		return err
	}
	defer mgr.release()
	id, freeID, err := hstring(aumid)
	if err != nil {
		return err
	}
	defer freeID()
	var notifier *comObject
	if hr := mgr.call(7, id, uintptr(unsafe.Pointer(&notifier))); hr != 0 { // CreateToastNotifierWithId
		return fmt.Errorf("CreateToastNotifier: 0x%08x", hr)
	}
	defer notifier.release()
	var setting int32
	if hr := notifier.call(8, uintptr(unsafe.Pointer(&setting))); hr == 0 && setting != 0 { // get_Setting: 0 = 켜짐
		return fmt.Errorf("%w (%d)", errToastDisabled, setting)
	}
	if hr := notifier.call(6, uintptr(unsafe.Pointer(toast))); hr != 0 { // Show
		return fmt.Errorf("Show: 0x%08x", hr)
	}
	return nil
}
