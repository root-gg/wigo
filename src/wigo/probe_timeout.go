package wigo

import (
	"fmt"
	"log"
	"sort"
)

// How long a probe is given to answer.
//
// Every probe used to get its interval minus a second, with no way to say
// otherwise. That is a ceiling, not a sensible wait : a probe wedged on an
// unreachable server holds on for the whole interval, and the result on screen
// stays the one from before the outage that entire time. A check_ssl_cert
// pitched at an hour would sit on a dead socket for fifty-nine minutes while
// the dashboard showed yesterday's OK.
//
// The useful answer -- "it did not answer" -- was available in ten seconds.
//
// So a timeout set here only ever shortens the wait. Asking for longer than
// the interval is refused rather than honoured : the scheduler fires a run
// every interval regardless, so a run allowed to outlive its interval would
// overlap the next one, two copies of the same probe would talk to the same
// thing at once, and whichever finished last would win. A probe that genuinely
// needs more time needs a longer interval, which is a different thing to
// change and the operator should be told so rather than obeyed into a mess.

// The shortest timeout worth honouring. Below this a probe is not being given
// a deadline, it is being told to fail, and it would fail the same way whether
// the thing it watches is healthy or not.
const minProbeTimeout = 1

// ProbeTimeout is how long the probe called name gets to answer, given the
// interval of the directory scheduling it.
//
// Without configuration this is the historical interval minus one second, so
// an install that says nothing behaves exactly as it did.
func ProbeTimeout(name string, interval int) int {
	timeout := interval - 1

	configured, ok := configuredProbeTimeout(name)
	if !ok {
		return timeout
	}

	// Refused, not clamped : clamping would silently do something other than
	// what the file asks, and the operator would go on believing the probe has
	// the time they gave it. The startup warning says which and why.
	if configured >= interval {
		return timeout
	}

	return configured
}

func configuredProbeTimeout(name string) (int, bool) {
	config := GetLocalWigo().GetConfig()
	if config == nil || config.ProbeTimeouts == nil {
		return 0, false
	}

	timeout, ok := config.ProbeTimeouts[name]
	if !ok || timeout < minProbeTimeout {
		return 0, false
	}

	return timeout, true
}

// CheckProbeTimeouts reports the configured timeouts that will not be applied,
// so they are said once at startup rather than discovered from a probe quietly
// behaving as though the file were empty.
//
// A timeout is checked against the interval that actually schedules the probe,
// which is what the probes directory says and not what the config file thinks.
func CheckProbeTimeouts() []string {
	config := GetLocalWigo().GetConfig()
	if config == nil || len(config.ProbeTimeouts) == 0 {
		return nil
	}

	problems := make([]string, 0)

	for _, name := range sortedProbeTimeoutNames(config.ProbeTimeouts) {
		timeout := config.ProbeTimeouts[name]

		if timeout < minProbeTimeout {
			problems = append(problems, fmt.Sprintf(
				"probe timeout for %s is %ds, which is not a deadline : ignored",
				name, timeout))
			continue
		}

		locations, err := FindProbeLocations(name)
		if err != nil {
			problems = append(problems, fmt.Sprintf(
				"probe timeout for %s : the probes directory could not be read : %s", name, err))
			continue
		}

		interval, _ := paceOf(locations)

		if interval == 0 {
			// A name that matches no probe at all is almost always a typo, and
			// the advice is different from the one for a probe that is installed
			// but runs nowhere. From the pace alone the two are the same zero,
			// and the locations only cover schedules -- an unscheduled probe
			// sits in examples, which is not one.
			if len(locations) == 0 && !probeExistsInExamples(probesRoot(), name) {
				problems = append(problems, fmt.Sprintf(
					"probe timeout for %s : no such probe, so nothing will use it", name))
				continue
			}

			problems = append(problems, fmt.Sprintf(
				"probe timeout for %s : nothing schedules that probe, so nothing will use it", name))
			continue
		}

		if timeout >= interval {
			problems = append(problems, fmt.Sprintf(
				"probe timeout for %s is %ds but it runs every %ds : ignored, since a run "+
					"outliving its interval would overlap the next one. Pitch it at a longer interval instead",
				name, timeout, interval))
		}
	}

	return problems
}

// LogProbeTimeoutProblems says at startup what will be ignored.
func LogProbeTimeoutProblems() {
	for _, problem := range CheckProbeTimeouts() {
		log.Printf("Config : %s\n", problem)
	}
}

// Named in a stable order : this is read at startup by a person comparing it
// to the file they just edited, and a map would shuffle it every boot.
func sortedProbeTimeoutNames(timeouts map[string]int) []string {
	names := make([]string, 0, len(timeouts))
	for name := range timeouts {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
