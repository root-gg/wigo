package wigo

import (
	"fmt"
	"log"
	"sort"
	"sync"
)

// What sits behind what.
//
// A router goes down and the forty hosts behind it stop answering. Each one is
// a separate discovery, each one is a separate message, and none of the forty
// is the news : the router is. Holding them back leaves one message that says
// what actually happened.
//
// Only the direct parents are looked at, which removes cycles by construction
// and loses nothing. A chain -- a host behind a switch behind a router -- still
// works : if the router is down the switch is unreachable too, so the switch is
// down, so the host behind it is held by its own direct parent. Reachability is
// transitive on its own and does not need walking.
//
// Nothing about the status changes. The hosts are still shown as down, still
// logged, still on their timeline. What is held is the interruption.

// DependencyConfig is one rule : this host, or every host of this group, is
// only reachable through DependsOn.
type DependencyConfig struct {
	// One or the other. A rule with both applies to both, which is a mistake
	// worth refusing rather than guessing at.
	Host  string
	Group string

	DependsOn []string
}

// Hosts whose problem was held back because something they sit behind was
// down.
//
// Kept so the recovery can be held too : forty hosts coming back produce forty
// "back to normal" messages about problems nobody was ever told about, which is
// the same storm arriving from the other side.
var heldByDependency = struct {
	sync.Mutex
	hosts map[string]bool
}{hosts: make(map[string]bool)}

func rememberHeldByDependency(hostname string) {
	heldByDependency.Lock()
	defer heldByDependency.Unlock()

	heldByDependency.hosts[hostname] = true
}

// takeHeldByDependency reports whether this host's problem was held, and
// forgets it. Reading it is what a recovery does, and a recovery happens once.
func takeHeldByDependency(hostname string) bool {
	heldByDependency.Lock()
	defer heldByDependency.Unlock()

	held := heldByDependency.hosts[hostname]
	delete(heldByDependency.hosts, hostname)

	return held
}

// dependenciesOf lists what a host sits behind, by name and by group.
//
// Both kinds of rule apply : a host named on its own and also covered by a
// group rule sits behind everything both name. Narrowing one with the other
// would be a way of writing an exception, and an exception nobody can see in
// the configuration is worse than a rule too wide.
func dependenciesOf(hostname string, group string) []string {
	config := GetLocalWigo().GetConfig()
	if config == nil {
		return nil
	}

	parents := make([]string, 0)
	seen := make(map[string]bool)

	for _, rule := range config.Dependencies {
		if rule.Host != hostname && (rule.Group == "" || rule.Group != group) {
			continue
		}

		for _, parent := range rule.DependsOn {
			// A host declared as its own parent would hold every message about
			// itself for ever, which is a configuration nobody meant to write.
			if parent == "" || parent == hostname || seen[parent] {
				continue
			}

			seen[parent] = true
			parents = append(parents, parent)
		}
	}

	return parents
}

// downParentOf returns the first thing this host sits behind that is not
// answering.
//
// A parent this wigo has never heard of counts as up, so the message goes out.
// That is the safe direction of the two : a typo then costs noise, where the
// other way round it would silently cost the alert.
func downParentOf(hostname string, group string) (string, bool) {
	for _, parent := range dependenciesOf(hostname, group) {
		found := GetLocalWigo().FindRemoteWigoByHostname(parent)
		if found == nil {
			continue
		}

		if !found.IsAlive {
			return parent, true
		}
	}

	return "", false
}

// heldByDependencies reports whether this notification is about a host that is
// only unreachable because something it sits behind is down.
func heldByDependencies(hostname string, group string, status int) (string, bool) {
	if hostname == "" {
		return "", false
	}

	// Back to normal. Held only if the problem it recovers from was held, so a
	// host nobody was told about does not get announced as fixed.
	if status <= 100 {
		if takeHeldByDependency(hostname) {
			return "", true
		}

		return "", false
	}

	parent, down := downParentOf(hostname, group)
	if !down {
		return "", false
	}

	rememberHeldByDependency(hostname)

	return parent, true
}

// DependencyProblems lists what is wrong with the rules as written.
//
// A rule naming a host this wigo does not watch does nothing at all, and doing
// nothing at all is exactly what a dependency looks like when it works.
func DependencyProblems() []string {
	config := GetLocalWigo().GetConfig()
	if config == nil {
		return nil
	}

	problems := make([]string, 0)

	for _, rule := range config.Dependencies {
		switch {
		case rule.Host == "" && rule.Group == "":
			problems = append(problems,
				"a dependency rule names neither a Host nor a Group and applies to nothing")
		case rule.Host != "" && rule.Group != "":
			problems = append(problems, fmt.Sprintf(
				"the dependency rule for host %q also names group %q ; it applies to both, which is probably not what was meant",
				rule.Host, rule.Group))
		}

		if len(rule.DependsOn) == 0 {
			problems = append(problems, fmt.Sprintf(
				"the dependency rule for %s depends on nothing and holds nothing back",
				ruleSubject(rule)))
		}
	}

	return problems
}

func ruleSubject(rule DependencyConfig) string {
	if rule.Host != "" {
		return fmt.Sprintf("host %q", rule.Host)
	}
	if rule.Group != "" {
		return fmt.Sprintf("group %q", rule.Group)
	}

	return "nothing"
}

// LogDependencyProblems says once what will never work.
func LogDependencyProblems() {
	for _, problem := range DependencyProblems() {
		log.Printf("Dependencies : %s", problem)
	}
}

// HeldByDependency is one host currently kept quiet, and what by.
type HeldByDependency struct {
	Host   string
	Behind string
}

// HeldHostsBehindSomethingDown lists what is currently held back this way.
//
// Read from the rules and what is answering right now rather than from what
// was held earlier : a host that has stopped being held should stop being
// listed, and the moment its parent answers again it has.
func HeldHostsBehindSomethingDown() []HeldByDependency {
	held := make([]HeldByDependency, 0)

	local := GetLocalWigo()
	if local == nil || local.config == nil {
		return held
	}

	for item := range local.RemoteWigos.IterBuffered() {
		remote, ok := item.Val.(*Wigo)
		if !ok || remote.IsAlive {
			continue
		}

		parent, down := downParentOf(remote.GetHostname(), remote.GetGroup())
		if !down {
			continue
		}

		held = append(held, HeldByDependency{Host: remote.GetHostname(), Behind: parent})
	}

	sort.Slice(held, func(i, j int) bool { return held[i].Host < held[j].Host })

	return held
}
