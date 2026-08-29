package wigo

import (
	"encoding/json"
	"strings"
	"testing"
)

func hostWith(group string, labels map[string]string) *Host {
	host := NewHost()
	host.Name = "db1"
	host.Group = group
	host.Labels = labels

	return host
}

// The group is what every existing wigo speaks, so it has to be reachable the
// new way too : otherwise filtering by label silently skips every host that
// predates labels, which is all of them on the day this ships.
func TestTheGroupTravelsAsALabel(t *testing.T) {
	labels := LabelsOf(hostWith("databases", nil))

	if labels[GroupLabel] != "databases" {
		t.Errorf("Got %+v, expected the group to be published as a label", labels)
	}
}

// Two places saying which group a host is in is one too many. The config file
// and the api would disagree the day they differ.
func TestAConfiguredGroupLabelIsIgnored(t *testing.T) {
	labels := LabelsOf(hostWith("databases", map[string]string{
		GroupLabel: "somewhere-else",
		"env":      "prod",
	}))

	if labels[GroupLabel] != "databases" {
		t.Errorf("Got %q, expected the real group to win", labels[GroupLabel])
	}
	if labels["env"] != "prod" {
		t.Errorf("Got %+v, expected the other labels to be kept", labels)
	}

	problems := strings.Join(CheckLabels(map[string]string{GroupLabel: "x"}), "\n")
	if !strings.Contains(problems, "derived from Group") {
		t.Errorf("It has to be said at startup, got %q", problems)
	}
}

// A host with no group at all must not gain an empty one : "group=" would match
// a selector nobody meant to write.
func TestAHostWithoutAGroupHasNoGroupLabel(t *testing.T) {
	labels := LabelsOf(hostWith("", map[string]string{"env": "prod"}))

	if _, present := labels[GroupLabel]; present {
		t.Errorf("Got %+v, expected no group label at all", labels)
	}
}

func TestASelectorMatchesEveryLabelItAsksFor(t *testing.T) {
	host := hostWith("databases", map[string]string{"env": "prod", "role": "db"})

	for _, text := range []string{"", "env=prod", "env=prod,role=db", "group=databases", "env=prod,group=databases"} {
		selector, err := ParseSelector(text)
		if err != nil {
			t.Fatalf("%q : unexpected error %s", text, err)
		}
		if !selector.Matches(host) {
			t.Errorf("%q should have matched %+v", text, LabelsOf(host))
		}
	}

	for _, text := range []string{"env=test", "env=prod,role=web", "dc=par1"} {
		selector, err := ParseSelector(text)
		if err != nil {
			t.Fatalf("%q : unexpected error %s", text, err)
		}
		if selector.Matches(host) {
			t.Errorf("%q should not have matched %+v", text, LabelsOf(host))
		}
	}
}

// A filter nobody filled in must not empty the screen.
func TestAnEmptySelectorMatchesEverything(t *testing.T) {
	selector, err := ParseSelector("  ")
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if !selector.Matches(hostWith("", nil)) {
		t.Errorf("An empty selector asks nothing, so it excludes nothing")
	}
}

// Answering nothing to a question probably meant as "either" would look like
// the hosts are gone, which is the one answer a monitoring tool must not give
// by accident.
func TestASelectorAskingTheSameKeyTwiceIsRefused(t *testing.T) {
	_, err := ParseSelector("env=prod,env=test")
	if err == nil {
		t.Fatalf("Expected it to be refused")
	}
	if !strings.Contains(err.Error(), "nothing can match") {
		t.Errorf("Got %q, expected it to say why", err)
	}
}

func TestASelectorThatIsNotKeyValueIsRefused(t *testing.T) {
	for _, text := range []string{"env", "env=", "=prod", "env=pr od", "en v=prod", "env=prod,"} {
		if _, err := ParseSelector(text); err == nil {
			t.Errorf("%q should have been refused", text)
		}
	}
}

// What a filter sees and what the startup warning says have to be the same
// thing, or an operator fixes a label the code already dropped.
func TestAnUnusableLabelIsBothDroppedAndReported(t *testing.T) {
	labels := LabelsOf(hostWith("databases", map[string]string{
		"env":     "prod",
		"bad key": "value",
		"role":    "two words",
	}))

	if _, present := labels["bad key"]; present {
		t.Errorf("Got %+v, expected the unusable key to be dropped", labels)
	}
	if _, present := labels["role"]; present {
		t.Errorf("Got %+v, expected the unusable value to be dropped", labels)
	}
	if labels["env"] != "prod" {
		t.Errorf("Got %+v, expected the usable one to survive", labels)
	}

	problems := strings.Join(CheckLabels(map[string]string{"bad key": "value", "role": "two words"}), "\n")
	if !strings.Contains(problems, "bad key") || !strings.Contains(problems, "two words") {
		t.Errorf("Both have to be named at startup, got %q", problems)
	}
}

func TestNothingIsReportedAboutUsableLabels(t *testing.T) {
	if problems := CheckLabels(map[string]string{"env": "prod", "dc": "par1"}); len(problems) != 0 {
		t.Errorf("Got %+v, expected nothing to say", problems)
	}
}

// A master running this version has to keep working with clients that do not.
// Both directions, since both happen during a fleet upgrade.
func TestLabelsSurviveAnOlderWigoOnEitherSide(t *testing.T) {
	// A client too old to know about labels sends a host without the field
	var fromOldClient Host
	if err := json.Unmarshal([]byte(`{"Name":"db1","Group":"databases","Status":100}`), &fromOldClient); err != nil {
		t.Fatalf("An older client's host has to decode : %s", err)
	}
	if fromOldClient.Labels != nil {
		t.Errorf("Got %+v, expected no labels rather than invented ones", fromOldClient.Labels)
	}
	if labels := LabelsOf(&fromOldClient); labels[GroupLabel] != "databases" {
		t.Errorf("Got %+v, expected its group to still be filterable", labels)
	}

	// And a master too old to know about labels ignores the field rather than
	// failing to decode the host at all
	encoded, err := json.Marshal(hostWith("databases", map[string]string{"env": "prod"}))
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	var asOldMasterSeesIt struct {
		Name   string
		Group  string
		Status int
	}
	if err := json.Unmarshal(encoded, &asOldMasterSeesIt); err != nil {
		t.Fatalf("An older master has to be able to decode this : %s", err)
	}
	if asOldMasterSeesIt.Group != "databases" {
		t.Errorf("Got %q, expected the group it has always read", asOldMasterSeesIt.Group)
	}
}

func TestASelectorRendersBackInAStableOrder(t *testing.T) {
	selector, err := ParseSelector("role=db,env=prod,dc=par1")
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if selector.String() != "dc=par1,env=prod,role=db" {
		t.Errorf("Got %q", selector.String())
	}
}

func labelledFleet(t *testing.T) *Wigo {
	t.Helper()

	local := setupTestWigo(t, "databases")
	local.LocalHost.Labels = map[string]string{"env": "prod", "role": "db"}

	web := newTestRemoteWigo("uuid-web", "web-1", "frontend")
	web.LocalHost.Labels = map[string]string{"env": "prod", "role": "web"}
	local.RemoteWigos.Set("uuid-web", web)

	// One that predates labels entirely : a group and nothing else
	staging := newTestRemoteWigo("uuid-old", "old-1", "frontend")
	local.RemoteWigos.Set("uuid-old", staging)

	// And one behind the first, since a label belongs to a host rather than to
	// where it sits in the tree
	deep := newTestRemoteWigo("uuid-deep", "deep-1", "databases")
	deep.LocalHost.Labels = map[string]string{"env": "test", "role": "db"}
	web.RemoteWigos.Set("uuid-deep", deep)

	return local
}

func TestHostsMatchingReachesTheWholeTree(t *testing.T) {
	fleet := labelledFleet(t)

	got := fleet.HostsMatching(mustSelect(t, "role=db"))
	if len(got) != 2 || got[0] != "deep-1" || got[1] != fleet.GetHostname() {
		t.Errorf("Got %v, expected the local host and the one two levels down", got)
	}

	if got := fleet.HostsMatching(mustSelect(t, "env=prod,role=web")); len(got) != 1 || got[0] != "web-1" {
		t.Errorf("Got %v, expected only web-1", got)
	}
}

// Every host that predates labels has a group and nothing else, which is the
// whole fleet on the day this ships.
func TestAHostWithOnlyAGroupIsStillReachable(t *testing.T) {
	fleet := labelledFleet(t)

	got := fleet.HostsMatching(mustSelect(t, "group=frontend"))
	if len(got) != 2 || got[0] != "old-1" || got[1] != "web-1" {
		t.Errorf("Got %v, expected both frontend hosts", got)
	}
}

func TestAnEmptySelectorReturnsEveryHost(t *testing.T) {
	fleet := labelledFleet(t)

	if got := fleet.HostsMatching(mustSelect(t, "")); len(got) != 4 {
		t.Errorf("Got %v, expected all four", got)
	}
}

// Offering a key nothing carries, or leaving out one somebody set, both make
// the filter on screen lie about the fleet.
func TestFleetLabelsCountsWhatIsActuallyThere(t *testing.T) {
	labels := labelledFleet(t).FleetLabels()

	if labels["env"]["prod"] != 2 || labels["env"]["test"] != 1 {
		t.Errorf("Got %+v for env", labels["env"])
	}
	if labels["role"]["db"] != 2 || labels["role"]["web"] != 1 {
		t.Errorf("Got %+v for role", labels["role"])
	}
	if labels[GroupLabel]["frontend"] != 2 || labels[GroupLabel]["databases"] != 2 {
		t.Errorf("Got %+v for the group label", labels[GroupLabel])
	}
	if _, present := labels["nothing"]; present {
		t.Errorf("Got a key nothing carries : %+v", labels)
	}
}

func TestTheHostsEndpointIsUnchangedWithoutTheParameter(t *testing.T) {
	fleet := labelledFleet(t)

	code, body := HttpRemotesHandler(nil, testRequest(t))
	if code != 200 {
		t.Fatalf("Code = %d", code)
	}

	names := make([]string, 0)
	if err := json.Unmarshal([]byte(body), &names); err != nil {
		t.Fatalf("Not valid json : %s", err)
	}
	if len(names) != len(fleet.ListRemoteWigosNames()) {
		t.Errorf("Got %v, expected the list it has always answered", names)
	}
}

func TestTheHostsEndpointNarrowsByLabel(t *testing.T) {
	labelledFleet(t)

	code, body := HttpRemotesHandler(nil, testRequestWithQuery(t, "labels=env=prod"))
	if code != 200 {
		t.Fatalf("Code = %d : %s", code, body)
	}

	names := make([]string, 0)
	if err := json.Unmarshal([]byte(body), &names); err != nil {
		t.Fatalf("Not valid json : %s", err)
	}
	if len(names) != 2 {
		t.Errorf("Got %v, expected the two prod hosts", names)
	}
}

// A selector nothing can match is a mistake worth refusing rather than
// answering with an empty list, which reads as the hosts being gone.
func TestABadSelectorIsRefused(t *testing.T) {
	labelledFleet(t)

	code, body := HttpRemotesHandler(nil, testRequestWithQuery(t, "labels=env=prod,env=test"))
	if code != 400 {
		t.Errorf("Code = %d, expected 400 : %s", code, body)
	}
}

func mustSelect(t *testing.T, text string) Selector {
	t.Helper()

	selector, err := ParseSelector(text)
	if err != nil {
		t.Fatalf("%q : %s", text, err)
	}

	return selector
}

// The api must not answer that a host carries a label the startup log says is
// ignored : both would be telling the truth about different things.
func TestWhatAHostPublishesIsAlreadyTheEffectiveSet(t *testing.T) {
	usable := UsableLabels(map[string]string{
		"env":      "prod",
		GroupLabel: "somewhere-else",
		"bad key":  "value",
		"role":     "two words",
	})

	if len(usable) != 1 || usable["env"] != "prod" {
		t.Errorf("Got %+v, expected only the usable one", usable)
	}
}

// What Groups cannot express : a host has one group, so a target interested in
// every database in par1 has to name them one by one and remember to come back
// when one is added.
func TestATargetRoutesOnLabels(t *testing.T) {
	target := AppriseTargetConfig{Labels: []string{"env=prod,role=db"}}

	if !target.Matches("db-1", "databases", map[string]string{"env": "prod", "role": "db", "dc": "par1"}) {
		t.Errorf("Expected the prod database to match")
	}
	if target.Matches("db-2", "databases", map[string]string{"env": "test", "role": "db"}) {
		t.Errorf("Every label of a selector has to match, not just one")
	}
	if target.Matches("web-1", "frontend", map[string]string{"env": "prod", "role": "web"}) {
		t.Errorf("Expected the prod web host not to match a database selector")
	}
}

// Any of the selectors, all of the labels within one : "prod or db" is two
// entries, "the prod databases" is one.
func TestSeveralSelectorsAreOred(t *testing.T) {
	target := AppriseTargetConfig{Labels: []string{"env=prod", "role=db"}}

	for _, labels := range []map[string]string{
		{"env": "prod", "role": "web"},
		{"env": "test", "role": "db"},
	} {
		if !target.Matches("host", "group", labels) {
			t.Errorf("%+v should have matched one of the two selectors", labels)
		}
	}

	if target.Matches("host", "group", map[string]string{"env": "test", "role": "web"}) {
		t.Errorf("Matching neither selector should not be notified")
	}
}

// Every existing target has to route exactly as it did : this is the whole
// reason labels were added next to Groups rather than in place of them.
func TestGroupsAndHostsStillRouteOnTheirOwn(t *testing.T) {
	target := AppriseTargetConfig{Groups: []string{"databases"}, Hosts: []string{"web-1"}}

	if !target.Matches("db-1", "databases", nil) {
		t.Errorf("A group filter must still work with no labels at all")
	}
	if !target.Matches("web-1", "frontend", nil) {
		t.Errorf("A host filter must still work with no labels at all")
	}
	if target.Matches("other", "frontend", nil) {
		t.Errorf("And still not match what it never matched")
	}
}

// A target with nothing at all would be notified about everything, which is
// what AppriseUrls is for.
func TestATargetWithNoFilterIsStillNeverNotified(t *testing.T) {
	target := AppriseTargetConfig{Urls: []string{"somewhere://"}}

	if target.Matches("db-1", "databases", map[string]string{"env": "prod"}) {
		t.Errorf("Expected a target without any filter never to match")
	}
}

// A selector that cannot be read must not quietly widen the target to
// everything : it is reported at startup and skipped when routing.
func TestAnUnreadableSelectorRoutesNothing(t *testing.T) {
	target := AppriseTargetConfig{Labels: []string{"nonsense", "env=prod,env=test"}}

	if target.Matches("db-1", "databases", map[string]string{"env": "prod"}) {
		t.Errorf("Expected an unusable selector to match nothing rather than everything")
	}

	// And the one that is readable, next to it, still routes
	target.Labels = append(target.Labels, "role=db")
	if !target.Matches("db-1", "databases", map[string]string{"role": "db"}) {
		t.Errorf("A usable selector next to a broken one has to keep working")
	}
}

func TestAppriseUrlsAreCollectedByLabel(t *testing.T) {
	config := &NotificationConfig{
		AppriseTargets: []AppriseTargetConfig{
			{Name: "dba", Urls: []string{"dba://"}, Labels: []string{"role=db"}},
			{Name: "oncall", Urls: []string{"oncall://"}, Labels: []string{"env=prod"}},
			{Name: "web", Urls: []string{"web://"}, Groups: []string{"frontend"}},
		},
	}

	urls := config.GetAppriseUrls("db-1", "databases", map[string]string{"role": "db", "env": "prod"}, false)
	if len(urls) != 2 {
		t.Fatalf("Got %+v, expected both label targets", urls)
	}

	// And nothing at all for a host matching none of them
	if urls := config.GetAppriseUrls("other", "none", map[string]string{"env": "test"}, false); len(urls) != 0 {
		t.Errorf("Got %+v, expected nothing", urls)
	}
}
