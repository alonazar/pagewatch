package main

import (
	"errors"
	"testing"
)

// fakeInstall replaces the real scheduler; it records the interval it was
// given, and fails if fail is set.
func fakeInstall(t *testing.T, fail bool) *[]int {
	t.Helper()
	var got []int
	old := installFn
	installFn = func(c *Config) error {
		got = append(got, c.interval())
		if fail {
			return errors.New("schtasks failed")
		}
		return nil
	}
	t.Cleanup(func() { installFn = old })
	return &got
}

func TestAskIntervalChoices(t *testing.T) {
	for answer, want := range map[string]int{"1": 10, "2": 30, "3": 60} {
		if got := askInterval(scripted(answer), 60); got != want {
			t.Errorf("answer %s: got %d minutes, want %d", answer, got, want)
		}
	}
}

func TestAskIntervalKeepsCustomValue(t *testing.T) {
	// 15 minutes was set in the settings file: a 4th option keeps it.
	if got := askInterval(scripted("4"), 15); got != 15 {
		t.Errorf("got %d, want 15 kept", got)
	}
	// With a standard value there is no 4th option, so "4" is refused and "1" picked.
	if got := askInterval(scripted("4", "1"), 60); got != 10 {
		t.Errorf("got %d, want 10", got)
	}
}

func TestChangeIntervalSavesAndReschedules(t *testing.T) {
	tempSettings(t)
	installed := fakeInstall(t, false)
	cfg := baseConfig(Alerts{Ntfy: ntfyA})

	changeInterval(scripted("1"), cfg) // every 10 minutes

	if cfg.CheckEveryMinutes != 10 {
		t.Fatalf("got %d minutes, want 10", cfg.CheckEveryMinutes)
	}
	if len(*installed) != 1 || (*installed)[0] != 10 {
		t.Errorf("schedule should be updated to 10 minutes, got %v", *installed)
	}
	saved, err := loadConfig()
	if err != nil || saved.CheckEveryMinutes != 10 {
		t.Fatalf("new interval should be saved: %v / %+v", err, saved)
	}
}

func TestChangeIntervalSameValueDoesNothing(t *testing.T) {
	tempSettings(t)
	installed := fakeInstall(t, false)
	cfg := baseConfig(Alerts{Ntfy: ntfyA}) // 0 means the default, every hour

	changeInterval(scripted("3"), cfg) // every hour

	if len(*installed) != 0 {
		t.Error("nothing should be rescheduled when the interval doesn't change")
	}
}

func TestChangeIntervalScheduleFailureIsReported(t *testing.T) {
	tempSettings(t)
	installed := fakeInstall(t, true)
	cfg := baseConfig(Alerts{Ntfy: ntfyA})

	changeInterval(scripted("2"), cfg)

	// The setting is saved; opening pagewatch again retries the schedule.
	saved, err := loadConfig()
	if err != nil || saved.CheckEveryMinutes != 30 || len(*installed) != 1 {
		t.Fatalf("expected the setting saved and one schedule attempt: %v / %+v / %v", err, saved, *installed)
	}
}

func TestIntervalChoicesAreValid(t *testing.T) {
	for _, m := range intervalChoices {
		c := baseConfig(Alerts{Ntfy: ntfyA})
		c.CheckEveryMinutes = m
		if err := c.validate(); err != nil {
			t.Errorf("%d minutes should be allowed: %v", m, err)
		}
	}
}
