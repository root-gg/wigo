package wigo

import "testing"

func newSelectionTestTree(t *testing.T) *Wigo {
	t.Helper()

	setupTestWigo(t, "network")
	LocalWigo.GlobalStatus = 100
	// Set directly rather than through AddOrUpdateProbe, which always writes to
	// the local host whatever it is called on.
	LocalWigo.LocalHost.Probes.Set("check_load", newTestProbe(LocalWigo.LocalHost, "check_load", 100))

	db1 := newTestRemoteWigo("uuid-db1", "db1", "databases")
	db1.GlobalStatus = 300
	db1.LocalHost.Probes.Set("check_disk", newTestProbe(db1.LocalHost, "check_disk", 300))
	db1.LocalHost.Probes.Set("check_load", newTestProbe(db1.LocalHost, "check_load", 100))

	web1 := newTestRemoteWigo("uuid-web1", "web1", "frontends")
	web1.GlobalStatus = 200
	web1.LocalHost.Probes.Set("check_http", newTestProbe(web1.LocalHost, "check_http", 200))

	LocalWigo.RemoteWigos.Set("db1", db1)
	LocalWigo.RemoteWigos.Set("web1", web1)

	return LocalWigo
}

// Asking about one group and being told about another one's outage is the kind
// of answer that costs somebody an hour.
func TestFilteringByGroupDropsTheOtherGroups(t *testing.T) {
	tree := newSelectionTestTree(t)
	tree.Apply(Selection{Group: "databases"})

	if _, kept := tree.RemoteWigos.Get("db1"); !kept {
		t.Errorf("The group that was asked for is gone")
	}
	if _, kept := tree.RemoteWigos.Get("web1"); kept {
		t.Errorf("A host from another group was kept")
	}
}

func TestFilteringByStatusDropsWhatIsFine(t *testing.T) {
	tree := newSelectionTestTree(t)
	tree.Apply(Selection{MinStatus: 300})

	if _, kept := tree.RemoteWigos.Get("db1"); !kept {
		t.Errorf("The critical host is gone")
	}
	if _, kept := tree.RemoteWigos.Get("web1"); kept {
		t.Errorf("A host below the threshold was kept")
	}
	if _, kept := tree.GetLocalHost().Probes.Get("check_load"); kept {
		t.Errorf("A probe below the threshold was kept")
	}
}

// The exit code has to come from what was shown. Reading the global status
// computed before filtering would report an outage the caller filtered out.
func TestTheWorstStatusIsReadFromWhatIsLeft(t *testing.T) {
	tree := newSelectionTestTree(t)

	if worst := tree.WorstStatus(); worst != 300 {
		t.Errorf("Got %d, expected the critical database", worst)
	}

	tree.Apply(Selection{Group: "frontends"})

	if worst := tree.WorstStatus(); worst != 200 {
		t.Errorf("Got %d, expected only the frontend warning", worst)
	}
}

// A host that is not answering says nothing about its probes, so its own status
// is the only thing carrying the news.
func TestAHostThatIsDownIsTheWorstStatus(t *testing.T) {
	tree := newSelectionTestTree(t)

	gone := newTestRemoteWigo("uuid-gone", "gone", "databases")
	gone.IsAlive = false
	gone.GlobalStatus = 500
	tree.RemoteWigos.Set("gone", gone)

	if worst := tree.WorstStatus(); worst != 500 {
		t.Errorf("Got %d, expected the host that stopped answering", worst)
	}
}

// The vocabulary every scheduler already speaks.
func TestStatusesMapOntoNagiosExitCodes(t *testing.T) {
	cases := map[int]int{
		100: ExitOk,
		101: ExitOk,
		199: ExitOk,
		200: ExitWarning,
		299: ExitWarning,
		300: ExitCritical,
		499: ExitCritical,
		500: ExitUnknown,
		999: ExitUnknown,
	}

	for status, expected := range cases {
		if got := NagiosExitCode(status); got != expected {
			t.Errorf("status %d exits %d, expected %d", status, got, expected)
		}
	}
}

// Names because nobody remembers that critical starts at 300, numbers because
// the scale is finer than its five names.
func TestAStatusThresholdIsANameOrANumber(t *testing.T) {
	cases := map[string]int{
		"":         0,
		"ok":       100,
		"WARNING":  200,
		" warn ":   200,
		"critical": 300,
		"crit":     300,
		"error":    500,
		"250":      250,
	}

	for written, expected := range cases {
		got, ok := ParseStatus(written)
		if !ok || got != expected {
			t.Errorf("ParseStatus(%q) = %d, %v ; expected %d", written, got, ok, expected)
		}
	}

	for _, written := range []string{"nope", "-1", "3.5"} {
		if _, ok := ParseStatus(written); ok {
			t.Errorf("ParseStatus(%q) was accepted", written)
		}
	}
}

// This host is a host like any other : asked about a group it is not in, it has
// nothing to say either. Without this, asking about a group it does not belong
// to still reported its own probes, and exited on their status.
func TestTheLocalHostIsFilteredByGroupToo(t *testing.T) {
	tree := newSelectionTestTree(t)
	tree.Apply(Selection{Group: "databases"})

	if _, kept := tree.GetLocalHost().Probes.Get("check_load"); kept {
		t.Errorf("The local host answered about a group it is not in")
	}

	tree = newSelectionTestTree(t)
	tree.Apply(Selection{Group: "network"})

	if _, kept := tree.GetLocalHost().Probes.Get("check_load"); !kept {
		t.Errorf("The local host was dropped from its own group")
	}
}
