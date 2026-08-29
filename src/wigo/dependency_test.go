package wigo

import (
	"strings"
	"testing"
)

func setupDependencyTest(t *testing.T, rules ...DependencyConfig) {
	t.Helper()

	setupTestWigo(t, "par1")
	LocalWigo.config.Dependencies = rules

	heldByDependency.Lock()
	heldByDependency.hosts = make(map[string]bool)
	heldByDependency.Unlock()
}

// remoteNamed adds a remote this wigo watches, up or down.
func remoteNamed(t *testing.T, hostname string, alive bool) *Wigo {
	t.Helper()

	remote := newTestRemoteWigo(hostname, hostname, "network")
	remote.IsAlive = alive

	LocalWigo.RemoteWigos.Set(hostname, remote)

	return remote
}

// The whole point : a router goes down and the forty hosts behind it stop
// answering, and none of the forty is the news.
func TestAHostBehindSomethingDownIsHeldBack(t *testing.T) {
	setupDependencyTest(t, DependencyConfig{Group: "par1", DependsOn: []string{"router-par1"}})
	remoteNamed(t, "router-par1", false)

	parent, held := heldByDependencies("web1", "par1", 500)
	if !held {
		t.Fatalf("web1 was reported while the router it sits behind is down")
	}
	if parent != "router-par1" {
		t.Errorf("Got %q, expected the message to name what is actually wrong", parent)
	}
}

// The router itself is the message that has to get out.
func TestTheThingEverythingSitsBehindIsStillReported(t *testing.T) {
	setupDependencyTest(t, DependencyConfig{Group: "par1", DependsOn: []string{"router-par1"}})
	remoteNamed(t, "router-par1", false)

	if _, held := heldByDependencies("router-par1", "network", 500); held {
		t.Errorf("The router was held back, so nobody is told anything at all")
	}
}

func TestNothingIsHeldWhileTheParentIsUp(t *testing.T) {
	setupDependencyTest(t, DependencyConfig{Group: "par1", DependsOn: []string{"router-par1"}})
	remoteNamed(t, "router-par1", true)

	if _, held := heldByDependencies("web1", "par1", 500); held {
		t.Errorf("web1 was held back while the router is answering")
	}
}

// Forty hosts coming back produce forty "back to normal" messages about
// problems nobody was ever told about : the same storm from the other side.
func TestTheRecoveryOfSomethingNeverReportedIsHeldToo(t *testing.T) {
	setupDependencyTest(t, DependencyConfig{Group: "par1", DependsOn: []string{"router-par1"}})
	remoteNamed(t, "router-par1", false)

	if _, held := heldByDependencies("web1", "par1", 500); !held {
		t.Fatalf("The problem was not held, so this tests nothing")
	}

	// The router is back, so is web1
	remoteNamed(t, "router-par1", true)

	if _, held := heldByDependencies("web1", "par1", 100); !held {
		t.Errorf("web1 was announced as fixed, and nobody knew it was broken")
	}

	// And only once : a later recovery is a real one
	if _, held := heldByDependencies("web1", "par1", 100); held {
		t.Errorf("A later recovery was held back too")
	}
}

// A host whose problem got through has a recovery worth sending.
func TestARealRecoveryGoesThrough(t *testing.T) {
	setupDependencyTest(t, DependencyConfig{Group: "par1", DependsOn: []string{"router-par1"}})
	remoteNamed(t, "router-par1", true)

	if _, held := heldByDependencies("web1", "par1", 100); held {
		t.Errorf("A recovery was held back for a problem that was reported")
	}
}

// A typo costs noise. The other way round it would silently cost the alert.
func TestAParentThisWigoNeverHeardOfCountsAsUp(t *testing.T) {
	setupDependencyTest(t, DependencyConfig{Group: "par1", DependsOn: []string{"rooter-par1"}})

	if _, held := heldByDependencies("web1", "par1", 500); held {
		t.Errorf("A rule naming a host nobody watches held a message back")
	}
}

// Both kinds of rule apply : narrowing one with the other would be an exception
// nobody can see in the configuration.
func TestHostAndGroupRulesAddUp(t *testing.T) {
	setupDependencyTest(t,
		DependencyConfig{Group: "par1", DependsOn: []string{"router-par1"}},
		DependencyConfig{Host: "web1", DependsOn: []string{"sw-2"}},
	)
	remoteNamed(t, "router-par1", true)
	remoteNamed(t, "sw-2", false)

	parent, held := heldByDependencies("web1", "par1", 500)
	if !held || parent != "sw-2" {
		t.Errorf("Got %q held=%v, expected the switch to hold it back", parent, held)
	}
}

// A host named as its own parent would hold every message about itself for
// ever, which is a configuration nobody meant to write.
func TestAHostCannotSitBehindItself(t *testing.T) {
	setupDependencyTest(t, DependencyConfig{Host: "web1", DependsOn: []string{"web1"}})
	remoteNamed(t, "web1", false)

	if _, held := heldByDependencies("web1", "par1", 500); held {
		t.Errorf("web1 was held back by itself")
	}
}

// Doing nothing at all is exactly what a dependency looks like when it works,
// so a rule that can never do anything has to say so.
func TestRulesThatCanNeverWorkAreReported(t *testing.T) {
	setupDependencyTest(t,
		DependencyConfig{DependsOn: []string{"router-par1"}},
		DependencyConfig{Host: "web1", Group: "par1", DependsOn: []string{"router-par1"}},
		DependencyConfig{Host: "web2"},
	)

	problems := DependencyProblems()
	if len(problems) != 3 {
		t.Fatalf("Got %d problems, expected 3 : %v", len(problems), problems)
	}

	joined := strings.Join(problems, " | ")
	for _, expected := range []string{"neither a Host nor a Group", "also names group", "depends on nothing"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("Expected %q in %s", expected, joined)
		}
	}
}

func TestARuleFreeWigoHoldsNothing(t *testing.T) {
	setupDependencyTest(t)
	remoteNamed(t, "router-par1", false)

	if _, held := heldByDependencies("web1", "par1", 500); held {
		t.Errorf("Something was held back with no rules configured")
	}
	if problems := DependencyProblems(); len(problems) != 0 {
		t.Errorf("Got %v, expected nothing to report", problems)
	}
}
