package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestShortcutReadByWindows writes a shortcut and asks Windows' own shell
// (through PowerShell, only in this test) what it points to.
func TestShortcutReadByWindows(t *testing.T) {
	dir := t.TempDir()
	// A folder name with a space and non-English letters, like many user folders.
	targetDir := filepath.Join(dir, "Program Files", "מבצע ü")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "pagewatch.exe")
	if err := os.WriteFile(target, []byte("MZ test"), 0o755); err != nil {
		t.Fatal(err)
	}
	lnk := filepath.Join(dir, "pagewatch.lnk")
	if err := writeShortcut(lnk, target, "Open pagewatch"); err != nil {
		t.Fatal(err)
	}

	// Write the answer to a UTF-8 file: the console would mangle Hebrew.
	outFile := filepath.Join(dir, "out.txt")
	// Shell.Application reads shortcuts with full Unicode support (WScript.Shell doesn't).
	ps := `$f = (New-Object -ComObject Shell.Application).NameSpace((Split-Path $env:PW_LNK)); ` +
		`$s = $f.ParseName((Split-Path $env:PW_LNK -Leaf)).GetLink; ` +
		`[IO.File]::WriteAllText($env:PW_OUT, "$($s.Path)|$($s.WorkingDirectory)|$($s.Description)")`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.Env = append(os.Environ(), "PW_LNK="+lnk, "PW_OUT="+outFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Windows could not read the shortcut: %v: %s", err, out)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(string(data), string(rune(0xFEFF)))), "|")
	if len(parts) != 3 {
		t.Fatalf("unexpected answer from Windows: %q", data)
	}
	// Windows may give back a folder in its long form ("runneradmin") where we
	// used the short 8.3 form ("RUNNER~1"); compare the long forms.
	if a, b := longPath(t, parts[0]), longPath(t, target); !strings.EqualFold(a, b) {
		t.Errorf("target: Windows reads %q, want %q", a, b)
	}
	if a, b := longPath(t, parts[1]), longPath(t, targetDir); !strings.EqualFold(a, b) {
		t.Errorf("working folder: Windows reads %q, want %q", a, b)
	}
	if parts[2] != "Open pagewatch" {
		t.Errorf("description: Windows reads %q", parts[2])
	}
}

// longPath turns any short 8.3 folder names in an existing path into their full names.
func longPath(t *testing.T, p string) string {
	t.Helper()
	in, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 1024)
	n, err := syscall.GetLongPathName(in, &buf[0], uint32(len(buf)))
	if err != nil || int(n) > len(buf) {
		t.Fatalf("GetLongPathName(%q): %v", p, err)
	}
	return syscall.UTF16ToString(buf[:n])
}
