//go:build !windows && !darwin

package main

import "errors"

// pagewatch supports Windows and Mac. On other systems, "pagewatch check"
// still works and can be run from cron.

const exeSuffix = ""

var errUnsupported = errors.New("automatic scheduling is only supported on Windows and Mac; run \"pagewatch check\" from cron instead")

func openConsole()                                  {}
func attachParentConsole()                          {}
func installSchedule(exe string, minutes int) error { return errUnsupported }
func removeSchedule() error                         { return nil }
func scheduleInstalled() bool                       { return false }
func scheduledInterval() int                        { return 0 }
func shortcutPath() string                          { return "" }
func installShortcut(exe string) error              { return nil }
func removeShortcut()                               {}

const reopenHint = `Run pagewatch again from a terminal to change settings.`
