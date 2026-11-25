package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// writeShortcut creates a Windows shortcut (.lnk) pointing at target, using
// Windows' own shortcut component (IShellLink) directly from pagewatch, the
// way installers do. It doesn't start PowerShell or any other hidden helper
// program, which antivirus software finds suspicious.
func writeShortcut(lnkPath, target, description string) error {
	// COM must be used from one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ole32 := syscall.NewLazyDLL("ole32.dll")
	coInit := ole32.NewProc("CoInitializeEx")
	coUninit := ole32.NewProc("CoUninitialize")
	coCreate := ole32.NewProc("CoCreateInstance")

	const coinitApartmentThreaded = 0x2
	hr, _, _ := coInit.Call(0, coinitApartmentThreaded)
	if int32(hr) >= 0 { // S_OK or S_FALSE: we must balance it
		defer coUninit.Call()
	}

	clsidShellLink := guid{0x00021401, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLinkW := guid{0x000214F9, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile := guid{0x0000010B, 0, 0, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}

	var link unsafe.Pointer
	const clsctxInprocServer = 0x1
	hr, _, _ = coCreate.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidShellLinkW)), uintptr(unsafe.Pointer(&link)))
	if int32(hr) < 0 {
		return fmt.Errorf("could not create shortcut object (0x%08X)", uint32(hr))
	}
	defer comCall(link, 2) // Release

	// IShellLinkW methods, by position in its interface.
	const (
		setDescription      = 7
		setWorkingDirectory = 9
		setPath             = 20
	)
	for _, step := range []struct {
		method int
		value  string
	}{
		{setPath, target},
		{setWorkingDirectory, filepath.Dir(target)},
		{setDescription, description},
	} {
		p, err := syscall.UTF16PtrFromString(step.value)
		if err != nil {
			return err
		}
		if hr := comCall(link, step.method, uintptr(unsafe.Pointer(p))); int32(hr) < 0 {
			return fmt.Errorf("could not set up shortcut (0x%08X)", uint32(hr))
		}
	}

	// Save through IPersistFile.
	var file unsafe.Pointer
	if hr := comCall(link, 0, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&file))); int32(hr) < 0 {
		return fmt.Errorf("could not save shortcut (0x%08X)", uint32(hr))
	}
	defer comCall(file, 2) // Release
	path, err := syscall.UTF16PtrFromString(lnkPath)
	if err != nil {
		return err
	}
	const save = 6
	if hr := comCall(file, save, uintptr(unsafe.Pointer(path)), 1); int32(hr) < 0 {
		return fmt.Errorf("could not save shortcut (0x%08X)", uint32(hr))
	}
	return nil
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// comCall calls method number i of a COM object's interface.
func comCall(obj unsafe.Pointer, i int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, i*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}
