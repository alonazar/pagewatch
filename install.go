package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// installedExe is where pagewatch copies itself, so the schedule keeps
// working even if the downloaded file is moved or deleted.
func installedExe() string {
	name := "pagewatch"
	if exeSuffix != "" {
		name += exeSuffix
	}
	return filepath.Join(appDir(), name)
}

// install copies this program to the app folder, (re)creates the schedule
// and the shortcut that lets the user open pagewatch again.
func install(cfg *Config) error {
	if err := os.MkdirAll(appDir(), 0o700); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	target := installedExe()
	if !samePath(self, target) {
		if err := copyFile(self, target); err != nil {
			return fmt.Errorf("could not copy pagewatch to %s (if a check is running right now, try again in a minute): %v", target, err)
		}
	}
	if err := installSchedule(target, cfg.interval()); err != nil {
		return fmt.Errorf("could not set up the schedule: %v", err)
	}
	if err := installShortcut(target); err != nil {
		logLine("Could not create the shortcut: " + err.Error())
	}
	return nil
}

func uninstall(deleteSettings bool) error {
	var errs []string
	if err := removeSchedule(); err != nil {
		errs = append(errs, "schedule: "+err.Error())
	}
	removeShortcut()

	self, _ := os.Executable()
	if !samePath(self, installedExe()) {
		_ = os.Remove(installedExe())
	}
	if deleteSettings {
		for _, f := range []string{configPath(), statePath(), logPath(), logPath() + ".old"} {
			_ = os.Remove(f)
		}
		_ = os.Remove(appDir()) // only succeeds if empty
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func samePath(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Remove(dst)
	return os.Rename(tmp, dst)
}

// logLine appends a timestamped line to pagewatch.log.
func logLine(line string) {
	_ = os.MkdirAll(appDir(), 0o700)
	if fi, err := os.Stat(logPath()); err == nil && fi.Size() > 512*1024 {
		_ = os.Rename(logPath(), logPath()+".old")
	}
	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s\n", time.Now().Format("2006-01-02 15:04"), line)
}

// cmdCheck is what the schedule runs.
func cmdCheck() int {
	cfg, err := loadConfig()
	if err != nil {
		logLine("Settings problem: " + err.Error())
		fmt.Println("Settings problem:", err)
		return 1
	}
	st := loadState()
	alert := func(title, msg, link string) (int, error) { return sendAlert(cfg.Alerts, title, msg, link) }
	for _, line := range runCheck(cfg, st, alert) {
		logLine(line)
		fmt.Println(line)
	}
	if err := saveState(st); err != nil {
		logLine("Could not save memory file: " + err.Error())
		return 1
	}
	return 0
}
