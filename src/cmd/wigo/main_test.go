package main

import (
	"container/list"
	"sort"
	"testing"
)

func stringList(names ...string) *list.List {
	l := list.New()
	for _, name := range names {
		l.PushBack(name)
	}
	return l
}

func listContent(l *list.List) []string {
	content := make([]string, 0, l.Len())
	for e := l.Front(); e != nil; e = e.Next() {
		content = append(content, e.Value.(string))
	}
	sort.Strings(content)
	return content
}

func equal(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReconcileProbesListAdds(t *testing.T) {
	current := stringList("check_load")
	found := stringList("check_load", "check_mdadm", "check_ntp")

	removed := reconcileProbesList("probes/60", current, found)

	if len(removed) != 0 {
		t.Errorf("Got %v removed, expected none", removed)
	}
	if want := []string{"check_load", "check_mdadm", "check_ntp"}; !equal(listContent(current), want) {
		t.Errorf("Got %v, expected %v", listContent(current), want)
	}
}

// Every probe that disappeared must be reported in a single pass. Removing an
// element while ranging over a container/list clears its next pointer, which
// used to end the loop early and leave all but the first deleted probe behind
// until the following cycle.
func TestReconcileProbesListRemovesAllInOnePass(t *testing.T) {
	current := stringList("check_load", "check_mdadm", "check_ntp", "check_uptime")
	found := stringList("check_uptime")

	removed := reconcileProbesList("probes/60", current, found)

	sort.Strings(removed)
	if want := []string{"check_load", "check_mdadm", "check_ntp"}; !equal(removed, want) {
		t.Errorf("Got %v removed, expected %v", removed, want)
	}
	if want := []string{"check_uptime"}; !equal(listContent(current), want) {
		t.Errorf("Got %v left, expected %v", listContent(current), want)
	}
}

func TestReconcileProbesListEmptiesDirectory(t *testing.T) {
	current := stringList("check_load", "check_mdadm")
	found := stringList()

	removed := reconcileProbesList("probes/60", current, found)

	sort.Strings(removed)
	if want := []string{"check_load", "check_mdadm"}; !equal(removed, want) {
		t.Errorf("Got %v removed, expected %v", removed, want)
	}
	if current.Len() != 0 {
		t.Errorf("Got %v left, expected an empty list", listContent(current))
	}
}

func TestReconcileProbesListAddsAndRemovesAtOnce(t *testing.T) {
	current := stringList("check_load", "check_mdadm")
	found := stringList("check_mdadm", "check_ntp")

	removed := reconcileProbesList("probes/60", current, found)

	if want := []string{"check_load"}; !equal(removed, want) {
		t.Errorf("Got %v removed, expected %v", removed, want)
	}
	if want := []string{"check_mdadm", "check_ntp"}; !equal(listContent(current), want) {
		t.Errorf("Got %v, expected %v", listContent(current), want)
	}
}

func TestReconcileProbesListNoChange(t *testing.T) {
	current := stringList("check_load", "check_mdadm")
	found := stringList("check_mdadm", "check_load")

	removed := reconcileProbesList("probes/60", current, found)

	if len(removed) != 0 {
		t.Errorf("Got %v removed, expected none", removed)
	}
	if current.Len() != 2 {
		t.Errorf("Got %d probes, expected 2", current.Len())
	}
}
