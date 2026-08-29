package wigo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTimeoutTest(t *testing.T, probes ...string) {
	t.Helper()

	setupTestWigo(t, "databases")
	LocalWigo.config.Global.ProbesDirectory = newTestProbesDirectory(t, probes...)
}

// The whole point of the default : an install that configures nothing must run
// exactly as it did before this existed.
func TestWithoutConfigurationAProbeKeepsItsIntervalMinusOne(t *testing.T) {
	setupTimeoutTest(t, "300/check_load")

	if timeout := ProbeTimeout("check_load", 300); timeout != 299 {
		t.Errorf("Got %d, expected 299", timeout)
	}

	// And a probe nobody named is not affected by one that was
	LocalWigo.config.ProbeTimeouts = map[string]int{"check_ntp": 10}

	if timeout := ProbeTimeout("check_load", 300); timeout != 299 {
		t.Errorf("Got %d, expected the untouched probe to keep 299", timeout)
	}
}

func TestAConfiguredTimeoutShortensTheWait(t *testing.T) {
	setupTimeoutTest(t, "300/check_load")
	LocalWigo.config.ProbeTimeouts = map[string]int{"check_load": 10}

	if timeout := ProbeTimeout("check_load", 300); timeout != 10 {
		t.Errorf("Got %d, expected 10", timeout)
	}
}

// A run allowed to outlive its interval would overlap the next one : two copies
// of the same probe talking to the same thing, and the later result winning
// whichever order they happen to finish in.
func TestATimeoutLongerThanTheIntervalIsRefused(t *testing.T) {
	setupTimeoutTest(t, "60/check_load")
	LocalWigo.config.ProbeTimeouts = map[string]int{"check_load": 120}

	if timeout := ProbeTimeout("check_load", 60); timeout != 59 {
		t.Errorf("Got %d, expected the interval minus one rather than the 120 asked for", timeout)
	}

	// Equal is refused too : the next run fires exactly then
	LocalWigo.config.ProbeTimeouts["check_load"] = 60

	if timeout := ProbeTimeout("check_load", 60); timeout != 59 {
		t.Errorf("Got %d, expected 59", timeout)
	}
}

func TestATimeoutThatIsNotADeadlineIsIgnored(t *testing.T) {
	setupTimeoutTest(t, "300/check_load")

	for _, nonsense := range []int{0, -1} {
		LocalWigo.config.ProbeTimeouts = map[string]int{"check_load": nonsense}

		if timeout := ProbeTimeout("check_load", 300); timeout != 299 {
			t.Errorf("A timeout of %d gave %d, expected the default 299", nonsense, timeout)
		}
	}
}

// Whatever is ignored has to be said once at startup. Discovering it from a
// probe behaving as though the file were empty is the failure this prevents.
func TestWhatWillBeIgnoredIsReported(t *testing.T) {
	setupTimeoutTest(t, "60/check_load", "300/check_ntp")

	// Present but linked from no interval directory : installed, unscheduled
	unscheduled := filepath.Join(LocalWigo.config.Global.ProbesDirectory, "examples", "check_ram")
	if err := os.WriteFile(unscheduled, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("Fail to create the unscheduled probe : %s", err)
	}

	LocalWigo.config.ProbeTimeouts = map[string]int{
		"check_load":    120,
		"check_ntp":     30,
		"check_ram":     10,
		"check_nothing": 10,
	}

	problems := strings.Join(CheckProbeTimeouts(), "\n")

	if !strings.Contains(problems, "check_load is 120s but it runs every 60s") {
		t.Errorf("The one longer than its interval has to be named : %s", problems)
	}
	if !strings.Contains(problems, "check_ram") || !strings.Contains(problems, "nothing schedules") {
		t.Errorf("An unscheduled probe has to be named : %s", problems)
	}
	if !strings.Contains(problems, "check_nothing") || !strings.Contains(problems, "no such probe") {
		t.Errorf("A probe that does not exist has to be named : %s", problems)
	}
	if strings.Contains(problems, "check_ntp") {
		t.Errorf("The one that will be honoured must not be complained about : %s", problems)
	}
}

func TestNothingIsReportedWhenNothingIsConfigured(t *testing.T) {
	setupTimeoutTest(t, "300/check_load")

	if problems := CheckProbeTimeouts(); len(problems) != 0 {
		t.Errorf("Got %+v, expected nothing to say", problems)
	}
}

// Read at startup by somebody comparing it to the file they just edited.
func TestTheReportComesInAStableOrder(t *testing.T) {
	setupTimeoutTest(t, "60/check_load")
	LocalWigo.config.ProbeTimeouts = map[string]int{
		"zzz_probe": 10,
		"aaa_probe": 10,
		"mmm_probe": 10,
	}

	first := CheckProbeTimeouts()
	for i := 0; i < 5; i++ {
		again := CheckProbeTimeouts()
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("The order moved between two reads :\n%v\n%v", first, again)
			}
		}
	}
}
