package wigo

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
)

// Labels : what a host is, said in more than one word.
//
// A host has exactly one Group, and a group has to be chosen : par1, or prod,
// or db, never the three at once. Whichever is picked, the other two questions
// stop being answerable -- "every database" and "everything in par1" are both
// reasonable things to ask of a monitoring tool, and one Group can only answer
// one of them.
//
// Labels are additive rather than a replacement. Group stays exactly as it is :
// it is the field every existing wigo speaks, on the wire and in the api, and
// a master running this version has to keep working with clients that do not.
// Nothing here changes what an older version sees.
//
// The group travels as a label too, derived rather than configured, so that one
// way of asking -- by label -- reaches everything including installs that have
// never heard of labels.

// The label a Group is published under, so filtering by label covers hosts
// that only have a group.
const GroupLabel = "group"

// Keys and values are matched against an allow list rather than escaped : they
// end up in urls, in log lines and on screen, and a label containing a comma or
// an equals sign would silently split into something else.
var validLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// IsValidLabel reports whether a key or a value may be used as a label.
func IsValidLabel(text string) bool {
	return len(text) <= 128 && validLabel.MatchString(text)
}

// LabelsOf is every label of a host, the derived group included.
//
// The group is derived, never taken from the configured labels : two places
// claiming to say which group a host is in is one place too many, and the api
// and the config file would disagree the day they differ.
func LabelsOf(host *Host) map[string]string {
	labels := make(map[string]string)

	if host == nil {
		return labels
	}

	// Filtered again here, not only where the config is read : these may have
	// arrived over the api from another wigo, whose config this one never saw.
	for key, value := range UsableLabels(host.Labels) {
		labels[key] = value
	}

	if host.Group != "" {
		labels[GroupLabel] = host.Group
	}

	return labels
}

// UsableLabels is what a host may publish : the configured labels minus what
// is refused.
//
// Applied where the config is read rather than where the labels are used, so
// the field on the host is already the effective set. Publishing the raw map
// would have the api answer that a host carries a label the startup log says is
// ignored, and both would be telling the truth about different things.
func UsableLabels(labels map[string]string) map[string]string {
	usable := make(map[string]string)

	for key, value := range labels {
		if key == GroupLabel || !IsValidLabel(key) || !IsValidLabel(value) {
			continue
		}
		usable[key] = value
	}

	return usable
}

// A Selector is a set of key=value a host has to match, all of them.
type Selector map[string]string

// ParseSelector reads "env=prod,role=db".
//
// Repeated keys are refused rather than resolved : "env=prod,env=test" cannot
// match anything, and answering nothing to a question that was probably meant
// as "either" would look like the hosts are gone.
func ParseSelector(text string) (Selector, error) {
	selector := make(Selector)

	if strings.TrimSpace(text) == "" {
		return selector, nil
	}

	for _, term := range strings.Split(text, ",") {
		key, value, found := strings.Cut(term, "=")
		if !found {
			return nil, fmt.Errorf("label selector %q is not key=value", term)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if !IsValidLabel(key) {
			return nil, fmt.Errorf("invalid label key %q", key)
		}
		if !IsValidLabel(value) {
			return nil, fmt.Errorf("invalid label value %q", value)
		}

		if previous, taken := selector[key]; taken {
			return nil, fmt.Errorf("label %q is asked for twice, as %q and %q, which nothing can match",
				key, previous, value)
		}

		selector[key] = value
	}

	return selector, nil
}

// Matches reports whether a host carries every label of the selector.
//
// An empty selector matches everything : it asks nothing, so nothing is
// excluded, and a filter nobody filled in must not empty the screen.
func (selector Selector) Matches(host *Host) bool {
	return selector.MatchesLabels(LabelsOf(host))
}

// MatchesLabels is the same question asked of labels already resolved, for
// callers that have them without having the host : a notification knows what it
// is about long after the host it came from was looked up.
func (selector Selector) MatchesLabels(labels map[string]string) bool {
	if len(selector) == 0 {
		return true
	}

	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}

	return true
}

// LabelsOfHostNamed is what the fleet currently knows about a host, by name.
//
// Looked up when it is needed rather than carried around : labels change when
// the host's config changes, and a copy taken when a problem started would
// route the recovery by what was true an hour ago.
func LabelsOfHostNamed(hostname string) map[string]string {
	local := GetLocalWigo()
	if local == nil {
		return map[string]string{}
	}

	if hostname == local.GetHostname() {
		return LabelsOf(local.LocalHost)
	}

	if remote := local.FindRemoteWigoByHostname(hostname); remote != nil {
		return LabelsOf(remote.LocalHost)
	}

	return map[string]string{}
}

// String renders a selector back, in a stable order.
func (selector Selector) String() string {
	terms := make([]string, 0, len(selector))
	for key, value := range selector {
		terms = append(terms, key+"="+value)
	}
	sort.Strings(terms)

	return strings.Join(terms, ",")
}

// CheckLabels reports what is wrong with the configured labels, so it is said
// once at startup rather than discovered from a filter that matches nothing.
func CheckLabels(labels map[string]string) []string {
	problems := make([]string, 0)

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if key == GroupLabel {
			problems = append(problems, fmt.Sprintf(
				"label %q is derived from Group and cannot be set here : ignored", GroupLabel))
			continue
		}

		if !IsValidLabel(key) {
			problems = append(problems, fmt.Sprintf(
				"label key %q is not usable : letters, digits, dot, dash and underscore, starting with a letter or a digit", key))
			continue
		}

		if !IsValidLabel(labels[key]) {
			problems = append(problems, fmt.Sprintf(
				"label %s has value %q, which is not usable : letters, digits, dot, dash and underscore, starting with a letter or a digit",
				key, labels[key]))
		}
	}

	return problems
}

// LogLabelProblems says at startup what will be ignored.
func LogLabelProblems() {
	config := GetLocalWigo().GetConfig()
	if config == nil {
		return
	}

	for _, problem := range CheckLabels(config.Labels) {
		log.Printf("Config : %s\n", problem)
	}
}

// HostsMatching is the names of every host carrying all of the selector's
// labels, this wigo included when it does.
//
// Recursive like the rest of the tree walks : a master of masters answers for
// everything it can see, since a label is a property of a host and not of where
// it happens to sit in the hierarchy.
func (this *Wigo) HostsMatching(selector Selector) []string {
	names := make([]string, 0)

	if this.Uuid == LocalWigo.Uuid && selector.Matches(this.LocalHost) {
		names = append(names, this.GetHostname())
	}

	for item := range this.RemoteWigos.IterBuffered() {
		remote := item.Val.(*Wigo)

		if selector.Matches(remote.LocalHost) {
			names = append(names, remote.GetHostname())
		}

		names = append(names, remote.HostsMatching(selector)...)
	}

	sort.Strings(names)

	return names
}

// FleetLabels is every label in use, with how many hosts carry each value.
//
// What the filter on screen is built from : offering a key nothing carries, or
// leaving out one somebody set, both make the filter lie about the fleet.
func (this *Wigo) FleetLabels() map[string]map[string]int {
	counts := make(map[string]map[string]int)

	this.countLabelsInto(counts)

	return counts
}

func (this *Wigo) countLabelsInto(counts map[string]map[string]int) {
	if this.Uuid == LocalWigo.Uuid {
		countLabels(counts, LabelsOf(this.LocalHost))
	}

	for item := range this.RemoteWigos.IterBuffered() {
		remote := item.Val.(*Wigo)

		countLabels(counts, LabelsOf(remote.LocalHost))
		remote.countLabelsInto(counts)
	}
}

func countLabels(counts map[string]map[string]int, labels map[string]string) {
	for key, value := range labels {
		if counts[key] == nil {
			counts[key] = make(map[string]int)
		}
		counts[key][value]++
	}
}
