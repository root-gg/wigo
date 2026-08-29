package wigo

import (
	"strconv"
	"strings"
)

// What a command line asks to be shown, and what it should exit with.
//
// Filtering happens on the tree before it is rendered rather than inside each
// renderer : the text summary and the json then agree by construction, and the
// exit code is computed from what was actually shown rather than from what was
// fetched. Asking about one group and being told about another one's outage is
// the kind of answer that costs somebody an hour.

// Selection narrows a fetched tree down to what was asked for.
type Selection struct {
	// Only hosts in this group. Empty keeps every group.
	Group string

	// Only probes and hosts at or above this status. Zero keeps everything.
	MinStatus int
}

// ParseStatus reads a status threshold written as a level name or a number.
//
// Names because nobody remembers that critical starts at 300, numbers because
// the scale is finer than its five names and somebody will want 250.
func ParseStatus(value string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return 0, true
	case "ok":
		return 100, true
	case "info":
		return 101, true
	case "warning", "warn":
		return 200, true
	case "critical", "crit":
		return 300, true
	case "error":
		return 500, true
	}

	status, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || status < 0 {
		return 0, false
	}

	return status, true
}

// Apply prunes the tree in place, keeping only what was asked for.
//
// In place because the caller owns this object : it deserialised it a moment
// ago from one http answer and nothing else is reading it.
func (this *Wigo) Apply(selection Selection) {
	if this.LocalHost != nil {
		// This host is a host like any other : asked about a group it is not
		// in, it has nothing to say either.
		outOfGroup := selection.Group != "" && this.LocalHost.Group != selection.Group

		for item := range this.LocalHost.Probes.IterBuffered() {
			probe, ok := item.Val.(*ProbeResult)
			if !ok || outOfGroup || probe.Status < selection.MinStatus {
				this.LocalHost.Probes.Remove(item.Key)
			}
		}
	}

	for item := range this.RemoteWigos.IterBuffered() {
		remote, ok := item.Val.(*Wigo)
		if !ok {
			this.RemoteWigos.Remove(item.Key)
			continue
		}

		// A remote is dropped for its own group, not for its children's : a
		// master watching several groups is not itself in any of them.
		if selection.Group != "" && remote.GetGroup() != selection.Group {
			// It may still be the way to something that does match, so it is
			// only dropped once nothing is left under it.
			remote.Apply(selection)
			if len(remote.RemoteWigos.Items()) == 0 {
				this.RemoteWigos.Remove(item.Key)
			}
			continue
		}

		remote.Apply(selection)

		if remote.GlobalStatus < selection.MinStatus && len(remote.RemoteWigos.Items()) == 0 {
			this.RemoteWigos.Remove(item.Key)
		}
	}
}

// WorstStatus is the worst thing left after filtering.
//
// What the exit code is built from. Read from what remains rather than from the
// global status computed before filtering, so asking about one group cannot
// report another one's outage.
func (this *Wigo) WorstStatus() int {
	worst := 0

	if this.LocalHost != nil {
		for item := range this.LocalHost.Probes.IterBuffered() {
			if probe, ok := item.Val.(*ProbeResult); ok && probe.Status > worst {
				worst = probe.Status
			}
		}
	}

	for item := range this.RemoteWigos.IterBuffered() {
		remote, ok := item.Val.(*Wigo)
		if !ok {
			continue
		}

		// A host that is not answering says nothing about its probes, so its
		// own status is the only thing that carries the news.
		if !remote.IsAlive && remote.GlobalStatus > worst {
			worst = remote.GlobalStatus
		}

		if status := remote.WorstStatus(); status > worst {
			worst = status
		}
	}

	return worst
}

// Nagios exit codes, which is the whole point of an exit code here : this is
// the vocabulary every scheduler already speaks.
const (
	ExitOk       = 0
	ExitWarning  = 1
	ExitCritical = 2
	ExitUnknown  = 3
)

// NagiosExitCode maps a wigo status onto what a monitoring plugin exits with.
//
// The mapping is wigo's own level names onto Nagios's, which line up except at
// the top : wigo's ERROR means a probe could not produce a result at all, and
// "could not tell" is exactly what Nagios calls UNKNOWN.
func NagiosExitCode(status int) int {
	switch {
	case status >= 500:
		return ExitUnknown
	case status >= 300:
		return ExitCritical
	case status >= 200:
		return ExitWarning
	default:
		return ExitOk
	}
}
