package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const exeSuffix = ".exe"
const taskName = "pagewatch"

// The Windows build is a "GUI" program (built with -H=windowsgui) so the
// hourly check runs without flashing a window. When the user opens it,
// we create a console window for the questions and menu.

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procAllocConsole = kernel32.NewProc("AllocConsole")
	procAttach       = kernel32.NewProc("AttachConsole")
	procSetTitle     = kernel32.NewProc("SetConsoleTitleW")
)

func reopenStdio() {
	if out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0); err == nil {
		os.Stdout = out
		os.Stderr = out
	}
	if in, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
		os.Stdin = in
	}
}

func openConsole() {
	r, _, _ := procAllocConsole.Call()
	if r != 0 { // a new console was created; otherwise we already have one
		reopenStdio()
	}
	if t, err := syscall.UTF16PtrFromString("pagewatch"); err == nil {
		procSetTitle.Call(uintptr(unsafe.Pointer(t)))
	}
}

func attachParentConsole() {
	const attachParentProcess = ^uintptr(0) // (DWORD)-1
	if r, _, _ := procAttach.Call(attachParentProcess); r != 0 {
		reopenStdio()
	}
}

func hideWindowCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

func installSchedule(exe string, minutes int) error {
	esc := func(s string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	start := time.Now().Add(2 * time.Minute).Format("2006-01-02T15:04:05")
	taskXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Checks web pages for words and sends alerts. Created by pagewatch; remove it from pagewatch's menu.</Description>
  </RegistrationInfo>
  <Triggers>
    <TimeTrigger>
      <Repetition>
        <Interval>PT%dM</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
      <StartBoundary>%s</StartBoundary>
      <Enabled>true</Enabled>
    </TimeTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings>
      <StopOnIdleEnd>false</StopOnIdleEnd>
      <RestartOnIdle>false</RestartOnIdle>
    </IdleSettings>
    <Enabled>true</Enabled>
    <ExecutionTimeLimit>PT10M</ExecutionTimeLimit>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
      <Arguments>check</Arguments>
    </Exec>
  </Actions>
</Task>
`, minutes, start, esc(exe))

	// schtasks wants the XML file in UTF-16 with a byte-order mark.
	u := utf16.Encode([]rune(taskXML))
	buf := []byte{0xFF, 0xFE}
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	tmp := filepath.Join(os.TempDir(), "pagewatch-task.xml")
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)

	out, err := hideWindowCmd("schtasks", "/Create", "/F", "/TN", taskName, "/XML", tmp).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeSchedule() error {
	if !scheduleInstalled() {
		return nil
	}
	out, err := hideWindowCmd("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func scheduleInstalled() bool {
	return hideWindowCmd("schtasks", "/Query", "/TN", taskName).Run() == nil
}

func shortcutPath() string {
	return filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\pagewatch.lnk`)
}

// installShortcut adds pagewatch to the Start menu.
func installShortcut(exe string) error {
	if err := os.MkdirAll(filepath.Dir(shortcutPath()), 0o755); err != nil {
		return err
	}
	ps := `$s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:PW_LNK); ` +
		`$s.TargetPath = $env:PW_EXE; $s.Description = 'Open pagewatch'; $s.Save()`
	cmd := hideWindowCmd("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.Env = append(os.Environ(), "PW_LNK="+shortcutPath(), "PW_EXE="+exe)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeShortcut() { _ = os.Remove(shortcutPath()) }

const reopenHint = `To open pagewatch again later, search for "pagewatch" in the Start menu.`
