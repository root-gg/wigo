package wigo

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// A probe that cannot make up its mind.
//
// Something oscillating OK to CRITICAL and back every minute sends two
// notifications a minute, all night. None of them is wrong, and together they
// are worse than useless : the real incident of the evening is buried under
// fifty messages about a link that keeps renegotiating.
//
// The answer is not to notify less about everything. It is to notice that this
// particular probe is behaving that way, say so once, and go quiet about it
// until it settles.
//
// What this deliberately does not do is hide anything. The probe keeps running,
// its status keeps being computed and displayed, and the first transitions are
// notified normally -- the threshold is only reached after several of them, so
// nobody learns about the problem for the first time from a flapping notice.

// FlapState is what is known about one probe's stability.
type FlapState struct {
	// How many status changes it has had inside the window.
	Transitions int

	// Whether it is currently considered to be flapping.
	Flapping bool

	// When it started flapping, zero when it is not.
	Since int64

	// Whether this very change is the one that tipped it over, or the one that
	// brought it back. Comparing Since to the clock would say the same thing
	// most of the time and the wrong thing when two changes land in the same
	// second, which is precisely what a flapping probe does.
	JustStarted bool
	JustSettled bool
}

// A probe is called flapping once it has changed status this many times inside
// the window, and is called settled again once it drops to half of that. The
// gap between the two is what keeps a probe sitting exactly on the threshold
// from flapping in and out of the flapping state itself.
const (
	defaultFlapWindow    = 3600
	defaultFlapThreshold = 5
)

var flapping = struct {
	sync.Mutex

	// Transition timestamps per host and probe, oldest first.
	transitions map[string][]int64

	// Which of them are currently considered to be flapping, and since when.
	since map[string]int64
}{
	transitions: make(map[string][]int64),
	since:       make(map[string]int64),
}

func flapKey(hostname string, probe string) string {
	return hostname + "\x00" + probe
}

func flapWindow() int64 {
	if window := GetLocalWigo().GetConfig().Notifications.FlapWindow; window > 0 {
		return int64(window)
	}

	return defaultFlapWindow
}

func flapThreshold() int {
	if threshold := GetLocalWigo().GetConfig().Notifications.FlapThreshold; threshold > 0 {
		return threshold
	}

	return defaultFlapThreshold
}

// RecordStatusChange notes that a probe changed status, and reports whether
// that makes it a flapping one.
//
// Every change is counted, including the ones too mild to be notified about :
// what is being measured is how steady the probe is, and a probe bouncing
// between OK and WARNING is exactly as unsteady as one bouncing between OK and
// CRITICAL, even though only the second one shouts about it.
func RecordStatusChange(hostname string, probe string) FlapState {
	if !GetLocalWigo().GetConfig().Notifications.FlapDetection {
		return FlapState{}
	}

	now := time.Now().Unix()
	window := flapWindow()
	threshold := flapThreshold()
	key := flapKey(hostname, probe)

	flapping.Lock()
	defer flapping.Unlock()

	kept := append(pruneLocked(key, now, window), now)
	flapping.transitions[key] = kept

	since, wasFlapping := flapping.since[key]

	// Hysteresis : it takes the full threshold to be called flapping, and
	// dropping to half of it to be called settled again. Without the gap a
	// probe sitting on the boundary would flap in and out of the state itself,
	// and each crossing would be worth a notification.
	if !wasFlapping && len(kept) >= threshold {
		flapping.since[key] = now
		return FlapState{Transitions: len(kept), Flapping: true, Since: now, JustStarted: true}
	}

	if wasFlapping && len(kept) <= threshold/2 {
		delete(flapping.since, key)
		return FlapState{Transitions: len(kept), JustSettled: true}
	}

	return FlapState{Transitions: len(kept), Flapping: wasFlapping, Since: since}
}

// pruneLocked drops the transitions that have fallen out of the window and
// returns what is left. The caller holds the lock.
func pruneLocked(key string, now int64, window int64) []int64 {
	recorded := flapping.transitions[key]

	kept := make([]int64, 0, len(recorded)+1)
	for _, at := range recorded {
		if at > now-window {
			kept = append(kept, at)
		}
	}
	flapping.transitions[key] = kept

	return kept
}

// settleLocked re-reads whether a probe is still unsteady, and stops calling it
// so when it is not.
//
// A probe that simply stops changing never comes back through the recording
// path, so without this it would keep its flapping badge for as long as nobody
// touched it -- which is the exact opposite of what the badge means. The state
// is derived from the window, so it has to be re-derived whenever it is read.
func settleLocked(key string, now int64, window int64, threshold int) FlapState {
	kept := pruneLocked(key, now, window)
	since, wasFlapping := flapping.since[key]

	if wasFlapping && len(kept) <= threshold/2 {
		delete(flapping.since, key)
		return FlapState{Transitions: len(kept)}
	}

	// Nothing left to remember about a probe that has been steady all window
	if !wasFlapping && len(kept) == 0 {
		delete(flapping.transitions, key)
	}

	return FlapState{Transitions: len(kept), Flapping: wasFlapping, Since: since}
}

// FlapStateOf reports what is known about a probe without recording a change.
func FlapStateOf(hostname string, probe string) FlapState {
	flapping.Lock()
	defer flapping.Unlock()

	return settleLocked(flapKey(hostname, probe), time.Now().Unix(), flapWindow(), flapThreshold())
}

// FlappingProbes lists what is currently unsteady, as "host/probe".
func FlappingProbes() []string {
	now := time.Now().Unix()
	window := flapWindow()
	threshold := flapThreshold()

	flapping.Lock()
	defer flapping.Unlock()

	keys := make([]string, 0, len(flapping.since))
	for key := range flapping.since {
		keys = append(keys, key)
	}

	names := make([]string, 0, len(keys))
	for _, key := range keys {
		if !settleLocked(key, now, window, threshold).Flapping {
			continue
		}

		hostname, probe, found := splitFlapKey(key)
		if !found {
			continue
		}
		names = append(names, fmt.Sprintf("%s/%s", hostname, probe))
	}

	return names
}

func splitFlapKey(key string) (string, string, bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return key[:i], key[i+1:], true
		}
	}

	return "", "", false
}

// forgetFlapping drops what is known about a probe, used when it goes away.
func forgetFlapping(hostname string, probe string) {
	key := flapKey(hostname, probe)

	flapping.Lock()
	defer flapping.Unlock()

	delete(flapping.transitions, key)
	delete(flapping.since, key)
}

// describeFlapping is what gets said once, when a probe is first called out.
func describeFlapping(hostname string, probe string, state FlapState) string {
	return fmt.Sprintf("Probe %s on host %s is flapping : %d status changes in the last %s. "+
		"Further changes are not notified until it settles.",
		probe, hostname, state.Transitions, time.Duration(flapWindow())*time.Second)
}

func logSettled(hostname string, probe string, state FlapState) {
	log.Printf("Probe %s on host %s has settled, %d status changes left in the window",
		probe, hostname, state.Transitions)
}
