package wigo

import (
	"testing"
)

func setupFlapTest(t *testing.T, threshold int, window int) {
	t.Helper()

	setupTestWigo(t, "databases")
	LocalWigo.config.Notifications.FlapDetection = true
	LocalWigo.config.Notifications.FlapThreshold = threshold
	LocalWigo.config.Notifications.FlapWindow = window

	flapping.Lock()
	flapping.transitions = make(map[string][]int64)
	flapping.since = make(map[string]int64)
	flapping.Unlock()
}

// A probe is called out once it has changed status often enough, and not before
// : nobody should learn about a problem for the first time from a flapping
// notice, so the early transitions go out normally.
func TestAProbeIsCalledOutAfterEnoughChanges(t *testing.T) {
	setupFlapTest(t, 4, 3600)

	for i := 1; i < 4; i++ {
		state := RecordStatusChange("db1", "check_load")
		if state.Flapping || state.JustStarted {
			t.Fatalf("Change %d should not have been enough : %+v", i, state)
		}
	}

	state := RecordStatusChange("db1", "check_load")
	if !state.Flapping || !state.JustStarted {
		t.Errorf("Got %+v, expected it to be called out on the fourth change", state)
	}
	if state.Transitions != 4 {
		t.Errorf("Got %d transitions, expected 4", state.Transitions)
	}
}

// Said once, then quiet : the whole point is not to send fifty messages.
func TestAProbeIsOnlyCalledOutOnce(t *testing.T) {
	setupFlapTest(t, 2, 3600)

	RecordStatusChange("db1", "check_load")
	first := RecordStatusChange("db1", "check_load")
	if !first.JustStarted {
		t.Fatalf("Got %+v, expected it to start flapping", first)
	}

	for i := 0; i < 5; i++ {
		state := RecordStatusChange("db1", "check_load")
		if state.JustStarted {
			t.Errorf("It should only be called out once, got %+v", state)
		}
		if !state.Flapping {
			t.Errorf("It should still be flapping, got %+v", state)
		}
	}
}

// Without hysteresis a probe sitting on the threshold would flap in and out of
// the flapping state itself, and each crossing would be worth a notification.
func TestSettlingTakesMoreThanCrossingBackTheThreshold(t *testing.T) {
	setupFlapTest(t, 4, 3600)

	for i := 0; i < 4; i++ {
		RecordStatusChange("db1", "check_load")
	}

	// Dropping to three, one below the threshold, is not settling
	dropTransitionsTo(t, "db1", "check_load", 3)
	state := RecordStatusChange("db1", "check_load")
	if !state.Flapping || state.JustSettled {
		t.Errorf("Got %+v, expected it to still be flapping just below the threshold", state)
	}

	// Half of it is
	dropTransitionsTo(t, "db1", "check_load", 1)
	state = RecordStatusChange("db1", "check_load")
	if state.Flapping || !state.JustSettled {
		t.Errorf("Got %+v, expected it to have settled", state)
	}
}

// The window is what makes this about how a probe behaves now, not about how it
// behaved last week.
func TestChangesOutsideTheWindowDoNotCount(t *testing.T) {
	setupFlapTest(t, 3, 60)

	for i := 0; i < 3; i++ {
		RecordStatusChange("db1", "check_load")
	}
	if !FlapStateOf("db1", "check_load").Flapping {
		t.Fatalf("It should be flapping")
	}

	ageTransitions(t, "db1", "check_load", 120)

	if count := FlapStateOf("db1", "check_load").Transitions; count != 0 {
		t.Errorf("Got %d transitions, expected the old ones to have fallen out", count)
	}

	// And the next change starts counting from one
	state := RecordStatusChange("db1", "check_load")
	if state.Transitions != 1 {
		t.Errorf("Got %+v, expected to be counting from one again", state)
	}
}

func TestFlapDetectionCanBeTurnedOff(t *testing.T) {
	setupFlapTest(t, 2, 3600)
	LocalWigo.config.Notifications.FlapDetection = false

	for i := 0; i < 10; i++ {
		if state := RecordStatusChange("db1", "check_load"); state.Flapping {
			t.Fatalf("Nothing should be called out when detection is off")
		}
	}
	if len(FlappingProbes()) != 0 {
		t.Errorf("Nothing should be listed")
	}
}

// Two probes are two independent stories, and so are the same probe on two
// hosts : one noisy link must not make the whole fleet look unsteady.
func TestProbesAreCountedApart(t *testing.T) {
	setupFlapTest(t, 3, 3600)

	for i := 0; i < 3; i++ {
		RecordStatusChange("db1", "check_load")
	}
	RecordStatusChange("db1", "smart")
	RecordStatusChange("db2", "check_load")

	if !FlapStateOf("db1", "check_load").Flapping {
		t.Errorf("db1/check_load should be flapping")
	}
	if FlapStateOf("db1", "smart").Flapping {
		t.Errorf("Another probe of the same host should not be")
	}
	if FlapStateOf("db2", "check_load").Flapping {
		t.Errorf("The same probe on another host should not be")
	}

	flappingProbes := FlappingProbes()
	if len(flappingProbes) != 1 || flappingProbes[0] != "db1/check_load" {
		t.Errorf("Got %+v, expected only db1/check_load", flappingProbes)
	}
}

// A probe that is gone must not come back flapping for changes it made in a
// previous life.
func TestForgettingAProbeThatWentAway(t *testing.T) {
	setupFlapTest(t, 2, 3600)

	RecordStatusChange("db1", "check_load")
	RecordStatusChange("db1", "check_load")
	if !FlapStateOf("db1", "check_load").Flapping {
		t.Fatalf("It should be flapping")
	}

	forgetFlapping("db1", "check_load")

	state := FlapStateOf("db1", "check_load")
	if state.Flapping || state.Transitions != 0 {
		t.Errorf("Got %+v, expected a clean slate", state)
	}
}

// Reading the state must not change it, or looking at a page would.
func TestFlapStateOfRecordsNothing(t *testing.T) {
	setupFlapTest(t, 2, 3600)

	RecordStatusChange("db1", "check_load")
	for i := 0; i < 5; i++ {
		FlapStateOf("db1", "check_load")
	}

	if state := FlapStateOf("db1", "check_load"); state.Transitions != 1 {
		t.Errorf("Got %+v, expected the single recorded change", state)
	}
}

// Probes are recorded from the goroutines that run them and read by whatever
// serves a page. Nothing here should race.
func TestFlappingIsSafeUnderConcurrency(t *testing.T) {
	setupFlapTest(t, 5, 3600)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			RecordStatusChange("db1", "check_load")
		}
		close(done)
	}()

	for i := 0; i < 200; i++ {
		FlapStateOf("db1", "check_load")
		FlappingProbes()
	}
	<-done
}

// dropTransitionsTo keeps only the most recent n, which is what the passage of
// time would do without waiting for it.
func dropTransitionsTo(t *testing.T, hostname string, probe string, n int) {
	t.Helper()

	flapping.Lock()
	defer flapping.Unlock()

	key := flapKey(hostname, probe)
	recorded := flapping.transitions[key]
	if len(recorded) > n {
		flapping.transitions[key] = recorded[len(recorded)-n:]
	}
}

func ageTransitions(t *testing.T, hostname string, probe string, seconds int64) {
	t.Helper()

	flapping.Lock()
	defer flapping.Unlock()

	key := flapKey(hostname, probe)
	aged := make([]int64, 0, len(flapping.transitions[key]))
	for _, at := range flapping.transitions[key] {
		aged = append(aged, at-seconds)
	}
	flapping.transitions[key] = aged
}

// A probe that simply stops changing never comes back through the recording
// path. Without re-deriving the state on read it would keep its flapping badge
// for as long as nobody touched it, which is the opposite of what it means.
func TestAProbeThatGoesQuietStopsBeingCalledFlapping(t *testing.T) {
	setupFlapTest(t, 4, 60)

	for i := 0; i < 4; i++ {
		RecordStatusChange("db1", "check_load")
	}
	if !FlapStateOf("db1", "check_load").Flapping {
		t.Fatalf("It should be flapping")
	}
	if len(FlappingProbes()) != 1 {
		t.Fatalf("It should be listed")
	}

	// Time passes and it changes nothing at all
	ageTransitions(t, "db1", "check_load", 120)

	if FlapStateOf("db1", "check_load").Flapping {
		t.Errorf("A probe steady for a whole window is not flapping any more")
	}
	if len(FlappingProbes()) != 0 {
		t.Errorf("Nor should it still be listed")
	}
}
