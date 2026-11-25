package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const exeSuffix = ""
const launchLabel = "io.github.pagewatch"

func openConsole()         {} // double-clicking on a Mac already opens Terminal
func attachParentConsole() {}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchLabel+".plist")
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func installSchedule(exe string, minutes int) error {
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>check</string>
  </array>
  <key>StartInterval</key><integer>%d</integer>
  <key>RunAtLoad</key><false/>
  <key>ProcessType</key><string>Background</string>
</dict>
</plist>
`, launchLabel, xmlEscape(exe), minutes*60)

	if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath(), []byte(plist), 0o644); err != nil {
		return err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain+"/"+launchLabel).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain, plistPath()).CombinedOutput(); err != nil {
		// Older macOS versions: fall back to the classic command.
		if out2, err2 := exec.Command("launchctl", "load", "-w", plistPath()).CombinedOutput(); err2 != nil {
			return fmt.Errorf("%v: %s %s", err, strings.TrimSpace(string(out)), strings.TrimSpace(string(out2)))
		}
	}
	return nil
}

func removeSchedule() error {
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", domain+"/"+launchLabel).Run()
	if err := os.Remove(plistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func scheduleInstalled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

// scheduledInterval reads back how often the schedule runs, in minutes
// (0 if unknown). Used by tests.
func scheduledInterval() int {
	data, err := os.ReadFile(plistPath())
	if err != nil {
		return 0
	}
	s := string(data)
	i := strings.Index(s, "<key>StartInterval</key><integer>")
	if i < 0 {
		return 0
	}
	s = s[i+len("<key>StartInterval</key><integer>"):]
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n / 60
}

func shortcutPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Applications", "pagewatch.command")
}

// installShortcut puts a "pagewatch" launcher in ~/Applications, which
// Spotlight finds and which opens pagewatch in Terminal.
func installShortcut(exe string) error {
	if err := os.MkdirAll(filepath.Dir(shortcutPath()), 0o755); err != nil {
		return err
	}
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(exe, "'", `'\''`) + "'\n"
	return os.WriteFile(shortcutPath(), []byte(script), 0o755)
}

func removeShortcut() { _ = os.Remove(shortcutPath()) }

const reopenHint = `To open pagewatch again later, press Cmd+Space and type "pagewatch".`
