package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	got := strings.TrimSpace(strings.TrimPrefix(string(data), string(rune(0xFEFF))))
	want := target + "|" + targetDir + "|Open pagewatch"
	if got != want {
		t.Fatalf("Windows reads the shortcut as\n  %q\nwant\n  %q", got, want)
	}
}
