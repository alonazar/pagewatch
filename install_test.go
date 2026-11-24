package main

import (
	"os"
	"testing"
)

// TestInstallSmoke really creates and removes the schedule on this computer.
// It only runs when PAGEWATCH_SMOKE=1 is set.
func TestInstallSmoke(t *testing.T) {
	if os.Getenv("PAGEWATCH_SMOKE") != "1" {
		t.Skip("set PAGEWATCH_SMOKE=1 to run")
	}
	if scheduleInstalled() {
		t.Skip("a pagewatch schedule already exists on this computer; not touching it")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	cfg := &Config{CheckEveryMinutes: 60}
	if err := install(cfg); err != nil {
		t.Fatal(err)
	}
	if !scheduleInstalled() {
		t.Fatal("schedule was not created")
	}
	if _, err := os.Stat(installedExe()); err != nil {
		t.Fatal("program was not copied:", err)
	}
	if _, err := os.Stat(shortcutPath()); err != nil {
		t.Error("shortcut was not created:", err)
	}
	if err := uninstall(true); err != nil {
		t.Fatal(err)
	}
	if scheduleInstalled() {
		t.Fatal("schedule was not removed")
	}
	if _, err := os.Stat(shortcutPath()); err == nil {
		t.Error("shortcut was not removed")
	}
}
