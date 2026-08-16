package wigo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWigoRecomputeGlobalStatus(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()

	host.Probes.Set("ok", newTestProbe(host, "ok", 100))
	host.Probes.Set("crit", newTestProbe(host, "crit", 300))

	wigo.RecomputeGlobalStatus()
	if wigo.GlobalStatus != 300 {
		t.Errorf("GlobalStatus = %d, expected 300", wigo.GlobalStatus)
	}

	// Only the local probes are taken into account, a remote wigo in error
	// does not degrade the global status
	remote := newTestRemoteWigo("uuid-1", "remote-1", "frontend")
	remote.GlobalStatus = 500
	wigo.RemoteWigos.Set(remote.Uuid, remote)

	wigo.RecomputeGlobalStatus()
	if wigo.GlobalStatus != 300 {
		t.Errorf("GlobalStatus = %d, expected the remote wigo to be ignored", wigo.GlobalStatus)
	}
}

func TestWigoGetters(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	if wigo.GetHostname() != "test-host" {
		t.Errorf("GetHostname() = %s", wigo.GetHostname())
	}
	if wigo.GetConfig() == nil {
		t.Errorf("GetConfig() returned nothing")
	}
	if wigo.GetOpenTsdb() != nil {
		t.Errorf("GetOpenTsdb() should be nil when OpenTSDB is disabled")
	}
}

func TestWigoAddOrUpdateRemoteWigo(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	remote := newTestRemoteWigo("uuid-1", "remote-1", "frontend")
	wigo.AddOrUpdateRemoteWigo(remote)

	tmp, ok := wigo.RemoteWigos.Get("uuid-1")
	if !ok {
		t.Fatalf("The remote wigo has not been added")
	}
	if tmp.(*Wigo).GetHostname() != "remote-1" {
		t.Errorf("Hostname = %s, expected remote-1", tmp.(*Wigo).GetHostname())
	}
	if tmp.(*Wigo).LastUpdate == 0 {
		t.Errorf("LastUpdate has not been stamped")
	}
}

// A wigo pushing our own uuid would make us monitor ourselves recursively.
func TestWigoAddOrUpdateRemoteWigoWithLocalUuid(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	wigo.AddOrUpdateRemoteWigo(newTestRemoteWigo(wigo.Uuid, "impostor", "frontend"))

	if wigo.RemoteWigos.Count() != 0 {
		t.Errorf("A remote wigo sharing our uuid has been added")
	}
}

// An already known remote wigo must not be duplicated deeper in the tree.
func TestWigoDeduplicate(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	known := newTestRemoteWigo("uuid-1", "remote-1", "frontend")
	wigo.RemoteWigos.Set(known.Uuid, known)

	proxy := newTestRemoteWigo("uuid-2", "proxy", "frontend")
	proxy.RemoteWigos.Set("uuid-1", newTestRemoteWigo("uuid-1", "remote-1", "frontend"))

	if err := wigo.Deduplicate(proxy); err != nil {
		t.Fatalf("Deduplicate() returned an error : %s", err)
	}
	if proxy.RemoteWigos.Count() != 0 {
		t.Errorf("The duplicated wigo has not been discarded")
	}
}

func TestWigoDeduplicateUuidMismatch(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	proxy := newTestRemoteWigo("uuid-2", "proxy", "frontend")
	// The map key and the wigo uuid disagree, the payload cannot be trusted
	proxy.RemoteWigos.Set("uuid-3", newTestRemoteWigo("uuid-1", "remote-1", "frontend"))

	if err := wigo.Deduplicate(proxy); err == nil {
		t.Errorf("Expected an error on a uuid mismatch")
	}
}

func TestWigoDeduplicateDropsOurself(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	proxy := newTestRemoteWigo("uuid-2", "proxy", "frontend")
	proxy.RemoteWigos.Set(wigo.Uuid, newTestRemoteWigo(wigo.Uuid, "test-host", "databases"))

	if err := wigo.Deduplicate(proxy); err != nil {
		t.Fatalf("Deduplicate() returned an error : %s", err)
	}
	if proxy.RemoteWigos.Count() != 0 {
		t.Errorf("The wigo sharing our uuid has not been discarded")
	}
}

func TestWigoFindRemoteWigoByHostname(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	proxy := newTestRemoteWigo("uuid-2", "proxy", "frontend")
	behind := newTestRemoteWigo("uuid-3", "behind-nat", "frontend")
	proxy.RemoteWigos.Set("behind-nat", behind)
	wigo.RemoteWigos.Set("proxy", proxy)

	if found := wigo.FindRemoteWigoByHostname("test-host"); found != wigo {
		t.Errorf("Looking up our own hostname should return ourself")
	}
	if found := wigo.FindRemoteWigoByHostname("proxy"); found != proxy {
		t.Errorf("The proxy has not been found")
	}
	// The lookup recurses through the whole tree
	if found := wigo.FindRemoteWigoByHostname("behind-nat"); found != behind {
		t.Errorf("The wigo behind the proxy has not been found")
	}
	if found := wigo.FindRemoteWigoByHostname("unknown"); found != nil {
		t.Errorf("Got %v, expected no result for an unknown hostname", found)
	}
}

func TestWigoListRemoteWigosNames(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	proxy := newTestRemoteWigo("uuid-2", "proxy", "frontend")
	proxy.RemoteWigos.Set("uuid-3", newTestRemoteWigo("uuid-3", "behind-nat", "frontend"))
	wigo.RemoteWigos.Set("uuid-2", proxy)

	names := wigo.ListRemoteWigosNames()

	for _, expected := range []string{"test-host", "proxy", "behind-nat"} {
		if !IsStringInArray(expected, names) {
			t.Errorf("Got %v, expected it to contain %s", names, expected)
		}
	}
}

func TestWigoListProbes(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()
	host.Probes.Set("load", newTestProbe(host, "load", 100))
	host.Probes.Set("disk", newTestProbe(host, "disk", 100))

	probes := wigo.ListProbes()

	if len(probes) != 2 || !IsStringInArray("load", probes) || !IsStringInArray("disk", probes) {
		t.Errorf("Got %v, expected load and disk", probes)
	}
}

func TestWigoEraseRemoteWigos(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	proxy := newTestRemoteWigo("uuid-2", "proxy", "frontend")
	proxy.RemoteWigos.Set("uuid-3", newTestRemoteWigo("uuid-3", "behind-nat", "frontend"))
	wigo.RemoteWigos.Set("uuid-2", proxy)

	// A depth of one keeps the direct remotes but drops what is behind them
	wigo.EraseRemoteWigos(2)

	if wigo.RemoteWigos.Count() != 1 {
		t.Errorf("The direct remote wigo should have been kept")
	}
	if proxy.RemoteWigos.Count() != 0 {
		t.Errorf("The wigo behind the proxy should have been erased")
	}

	// A depth of one erases everything
	wigo.EraseRemoteWigos(1)
	if wigo.RemoteWigos.Count() != 0 {
		t.Errorf("All the remote wigos should have been erased")
	}
}

func TestWigoListGroupsNames(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	proxy := newTestRemoteWigo("uuid-2", "proxy", "frontend")
	proxy.RemoteWigos.Set("uuid-3", newTestRemoteWigo("uuid-3", "behind-nat", "frontend"))
	wigo.RemoteWigos.Set("uuid-2", proxy)
	wigo.RemoteWigos.Set("uuid-4", newTestRemoteWigo("uuid-4", "no-group", ""))

	groups := wigo.ListGroupsNames()

	// Groups are deduplicated and the empty group is skipped
	if len(groups) != 2 {
		t.Fatalf("Got %v, expected two groups", groups)
	}
	if !IsStringInArray("databases", groups) || !IsStringInArray("frontend", groups) {
		t.Errorf("Got %v, expected databases and frontend", groups)
	}
}

func TestWigoGroupSummary(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	wigo.GlobalStatus = 100

	remote := newTestRemoteWigo("uuid-2", "db-2", "databases")
	remote.GlobalStatus = 300
	remote.LocalHost.Probes.Set("load", newTestProbe(remote.LocalHost, "load", 300))
	remote.LocalHost.RecomputeStatus()
	wigo.RemoteWigos.Set("uuid-2", remote)

	wigo.RemoteWigos.Set("uuid-3", newTestRemoteWigo("uuid-3", "web-1", "frontend"))

	summaries, status := wigo.GroupSummary("databases")

	if len(summaries) != 2 {
		t.Fatalf("Got %d summaries, expected the local host and db-2", len(summaries))
	}
	// The group status is the worst status of its members
	if status != 300 {
		t.Errorf("Status = %d, expected 300", status)
	}

	if summaries, status := wigo.GroupSummary("unknown"); len(summaries) != 0 || status != 0 {
		t.Errorf("Got %v and status %d, expected nothing for an unknown group", summaries, status)
	}
}

func TestWigoDisabledProbes(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	if wigo.IsProbeDisabled("load") {
		t.Errorf("No probe should be disabled yet")
	}

	wigo.DisableProbe("load")
	if !wigo.IsProbeDisabled("load") {
		t.Errorf("The load probe should be disabled")
	}

	// Disabling twice must not duplicate the entry
	wigo.DisableProbe("load")
	if wigo.GetDisabledProbes().Len() != 1 {
		t.Errorf("Got %d disabled probes, expected 1", wigo.GetDisabledProbes().Len())
	}

	// An empty name is ignored
	wigo.DisableProbe("")
	if wigo.GetDisabledProbes().Len() != 1 {
		t.Errorf("An empty probe name has been disabled")
	}
}

func TestWigoToJsonStringAndBack(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()
	host.Probes.Set("load", newTestProbe(host, "load", 300))
	host.RecomputeStatus()
	wigo.RecomputeGlobalStatus()

	serialized, err := wigo.ToJsonString()
	if err != nil {
		t.Fatalf("ToJsonString() returned an error : %s", err)
	}

	decoded, err := NewWigoFromJson([]byte(serialized), 0)
	if err != nil {
		t.Fatalf("NewWigoFromJson() returned an error : %s", err)
	}

	if decoded.GetHostname() != "test-host" {
		t.Errorf("Hostname = %s, expected test-host", decoded.GetHostname())
	}
	if decoded.GetLocalHost().Group != "databases" {
		t.Errorf("Group = %s, expected databases", decoded.GetLocalHost().Group)
	}
	if decoded.GlobalStatus != 300 {
		t.Errorf("GlobalStatus = %d, expected 300", decoded.GlobalStatus)
	}

	tmp, ok := decoded.GetLocalHost().Probes.Get("load")
	if !ok {
		t.Fatalf("The load probe is missing after the round trip")
	}
	// Parent hosts are not serialized, they are restored on decoding
	if tmp.(*ProbeResult).GetHost() != decoded.GetLocalHost() {
		t.Errorf("The probe parent host has not been restored")
	}
	if decoded.GetLocalHost().GetParentWigo() != decoded {
		t.Errorf("The local host parent wigo has not been restored")
	}
}

// Wigos sent by an old version have no Hostname field, it is taken from the
// local host name instead.
func TestNewWigoFromJsonWithoutHostname(t *testing.T) {

	decoded, err := NewWigoFromJson([]byte(`{"LocalHost":{"Name":"legacy-host","Probes":{}}}`), 0)
	if err != nil {
		t.Fatalf("NewWigoFromJson() returned an error : %s", err)
	}

	if decoded.GetHostname() != "legacy-host" {
		t.Errorf("Hostname = %s, expected legacy-host", decoded.GetHostname())
	}
}

func TestNewWigoFromInvalidJson(t *testing.T) {

	if _, err := NewWigoFromJson([]byte("this is not json"), 0); err == nil {
		t.Errorf("Expected an error for an invalid payload")
	}
}

func TestNewWigoFromJsonErasesRemotesBeyondDepth(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	proxy := newTestRemoteWigo("uuid-2", "proxy", "frontend")
	proxy.RemoteWigos.Set("uuid-3", newTestRemoteWigo("uuid-3", "behind-nat", "frontend"))
	wigo.RemoteWigos.Set("uuid-2", proxy)

	serialized, err := wigo.ToJsonString()
	if err != nil {
		t.Fatalf("ToJsonString() returned an error : %s", err)
	}

	decoded, err := NewWigoFromJson([]byte(serialized), 1)
	if err != nil {
		t.Fatalf("NewWigoFromJson() returned an error : %s", err)
	}

	if decoded.RemoteWigos.Count() != 0 {
		t.Errorf("The remote wigos should have been erased at depth 1")
	}
}

func TestWigoUpAndDown(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	remote := newTestRemoteWigo("uuid-2", "db-2", "databases")
	remote.Down()

	if remote.IsAlive || remote.GlobalStatus != 999 || remote.GlobalMessage != "DOWN" {
		t.Errorf("Got IsAlive %v, status %d and message %q", remote.IsAlive, remote.GlobalStatus, remote.GlobalMessage)
	}
	if remote.GetLocalHost().Status != 999 {
		t.Errorf("The local host status should follow the wigo status")
	}

	// The notification carries the host and the group so it can be filtered
	notification := <-Channels.ChanCallbacks
	if notification.GetHostname() != "db-2" || notification.GetGroup() != "databases" {
		t.Errorf("Got host %q and group %q", notification.GetHostname(), notification.GetGroup())
	}
	if !strings.Contains(notification.GetMessage(), "DOWN") {
		t.Errorf("Message = %s, expected a DOWN notification", notification.GetMessage())
	}

	remote.Up()
	if !remote.IsAlive || remote.GlobalMessage != "UP" {
		t.Errorf("Got IsAlive %v and message %q", remote.IsAlive, remote.GlobalMessage)
	}

	notification = <-Channels.ChanCallbacks
	if notification.GetHostname() != "db-2" || notification.GetGroup() != "databases" {
		t.Errorf("Got host %q and group %q", notification.GetHostname(), notification.GetGroup())
	}

	_ = wigo
}

func TestWigoCompareTwoWigosAndRaiseNotifications(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	wigo.GetConfig().Notifications.OnProbeChange = true

	oldWigo := newTestRemoteWigo("uuid-2", "db-2", "databases")
	oldWigo.LocalHost.Probes.Set("load", newTestProbe(oldWigo.LocalHost, "load", 100))
	oldWigo.LocalHost.Probes.Set("gone", newTestProbe(oldWigo.LocalHost, "gone", 300))

	newWigo := newTestRemoteWigo("uuid-2", "db-2", "databases")
	newWigo.LocalHost.Probes.Set("load", newTestProbe(newWigo.LocalHost, "load", 300))
	newWigo.LocalHost.Probes.Set("fresh", newTestProbe(newWigo.LocalHost, "fresh", 100))

	wigo.CompareTwoWigosAndRaiseNotifications(oldWigo, newWigo)

	// Only the status change of load reaches the callbacks : appearing and
	// disappearing probes are logged but never sent, the send condition needs
	// both an old and a new probe
	messages := make([]string, 0)
	for len(Channels.ChanCallbacks) > 0 {
		messages = append(messages, (<-Channels.ChanCallbacks).GetMessage())
	}

	if len(messages) != 1 {
		t.Fatalf("Got %v, expected only the load status change to be sent", messages)
	}
	if !strings.Contains(messages[0], "load") {
		t.Errorf("Got %q, expected a notification about the load probe", messages[0])
	}
}

// A wigo that just came back must not flood with notifications for probes that
// disappeared while it was down.
func TestWigoCompareTwoWigosWithDeadNewWigo(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	wigo.GetConfig().Notifications.OnProbeChange = true

	oldWigo := newTestRemoteWigo("uuid-2", "db-2", "databases")
	oldWigo.LocalHost.Probes.Set("gone", newTestProbe(oldWigo.LocalHost, "gone", 300))

	newWigo := newTestRemoteWigo("uuid-2", "db-2", "databases")
	newWigo.IsAlive = false

	wigo.CompareTwoWigosAndRaiseNotifications(oldWigo, newWigo)

	if len(Channels.ChanCallbacks) != 0 {
		t.Errorf("Got %d notifications, expected none from a dead wigo", len(Channels.ChanCallbacks))
	}
}

func TestWigoSetParentHostsInProbes(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	probe := newTestProbe(nil, "load", 100)
	wigo.GetLocalHost().Probes.Set("load", probe)

	remote := newTestRemoteWigo("uuid-2", "db-2", "databases")
	remoteProbe := newTestProbe(nil, "disk", 100)
	remote.LocalHost.Probes.Set("disk", remoteProbe)
	wigo.RemoteWigos.Set("uuid-2", remote)

	wigo.SetParentHostsInProbes()

	if probe.GetHost() != wigo.GetLocalHost() {
		t.Errorf("The local probe has not been attached to the local host")
	}
	// The whole tree is walked, not only the local host
	if remoteProbe.GetHost() != remote.GetLocalHost() {
		t.Errorf("The remote probe has not been attached to its host")
	}
}

func TestWigoGenerateSummary(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	wigo.Version = "0.74.0"
	host := wigo.GetLocalHost()
	host.Probes.Set("load", newTestProbe(host, "load", 300))
	host.RecomputeStatus()
	wigo.RecomputeGlobalStatus()

	summary := wigo.GenerateSummary(false)

	for _, expected := range []string{"0.74.0", "test-host", "load"} {
		if !strings.Contains(summary, expected) {
			t.Errorf("The summary does not mention %q :\n%s", expected, summary)
		}
	}
}

func TestWigoGenerateSummaryOnlyErrors(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()
	host.Probes.Set("load", newTestProbe(host, "load", 100))
	host.RecomputeStatus()

	// Nothing is wrong, the local probes section is skipped
	if summary := wigo.GenerateSummary(true); strings.Contains(summary, "Local probes") {
		t.Errorf("The local probes should not be listed :\n%s", summary)
	}
}

func TestWigoJsonHidesPrivateFields(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	serialized, err := wigo.ToJsonString()
	if err != nil {
		t.Fatalf("ToJsonString() returned an error : %s", err)
	}

	fields := make(map[string]interface{})
	if err := json.Unmarshal([]byte(serialized), &fields); err != nil {
		t.Fatalf("The serialized wigo is not valid json : %s", err)
	}

	for _, expected := range []string{"Uuid", "Hostname", "LocalHost", "RemoteWigos", "GlobalStatus"} {
		if _, ok := fields[expected]; !ok {
			t.Errorf("The %s field is missing from the payload", expected)
		}
	}
	// The configuration holds credentials and must never leave the process
	for _, unexpected := range []string{"config", "Config", "sqlLiteConn"} {
		if _, ok := fields[unexpected]; ok {
			t.Errorf("The %s field must not be serialized", unexpected)
		}
	}
}
