package wigo

import (
	"testing"
)

func TestHostRecomputeStatus(t *testing.T) {

	host := NewHost()

	// A brand new host is optimistic
	if host.Status != 100 {
		t.Errorf("A new host status = %d, expected 100", host.Status)
	}

	host.Probes.Set("ok", newTestProbe(host, "ok", 100))
	host.Probes.Set("warn", newTestProbe(host, "warn", 200))
	host.Probes.Set("crit", newTestProbe(host, "crit", 300))

	// The host status is the worst status of its probes
	host.RecomputeStatus()
	if host.Status != 300 {
		t.Errorf("Status = %d, expected 300", host.Status)
	}

	host.Probes.Remove("crit")
	host.RecomputeStatus()
	if host.Status != 200 {
		t.Errorf("Status = %d, expected 200", host.Status)
	}

	// Without any probe there is nothing to report, which is not an error
	host.Probes.Remove("ok")
	host.Probes.Remove("warn")
	host.RecomputeStatus()
	if host.Status != 100 {
		t.Errorf("Status = %d, expected 100", host.Status)
	}
}

func TestHostRecomputeStatusWithoutProbeIsNotAnError(t *testing.T) {

	host := NewHost()

	// A host that never ran a probe must not be reported as being in error :
	// every status below 100 is an error level.
	host.RecomputeStatus()
	if host.Status != 100 {
		t.Errorf("Status = %d, expected 100 for a host without any probe", host.Status)
	}

	// A probe below 100 is still an error and must not be masked
	host.Probes.Set("broken", newTestProbe(host, "broken", 50))
	host.RecomputeStatus()
	if host.Status != 50 {
		t.Errorf("Status = %d, expected 50 : a sub-100 probe status must win", host.Status)
	}
}

func TestHostGetErrorsProbesList(t *testing.T) {

	host := NewHost()
	host.Probes.Set("ok", newTestProbe(host, "ok", 100))
	host.Probes.Set("info", newTestProbe(host, "info", 101))
	host.Probes.Set("crit", newTestProbe(host, "crit", 300))

	errors := host.GetErrorsProbesList()

	// Everything above 100 counts as an error, including the INFO range
	if len(errors) != 2 {
		t.Fatalf("Got %v, expected two probes in error", errors)
	}
	if !IsStringInArray("info", errors) || !IsStringInArray("crit", errors) {
		t.Errorf("Got %v, expected info and crit", errors)
	}
}

func TestHostGetErrorsProbesListWithoutError(t *testing.T) {

	host := NewHost()
	host.Probes.Set("ok", newTestProbe(host, "ok", 100))

	// The empty list must not be nil, it is serialized in notifications
	if errors := host.GetErrorsProbesList(); errors == nil || len(errors) != 0 {
		t.Errorf("Got %v, expected an empty list", errors)
	}
}

func TestHostGetSummary(t *testing.T) {

	host := NewHost()
	host.Name = "db-1"
	host.Probes.Set("load", newTestProbe(host, "load", 300))
	host.RecomputeStatus()

	summary := host.GetSummary()

	if summary.Name != "db-1" {
		t.Errorf("Name = %s, expected db-1", summary.Name)
	}
	if summary.Status != 300 {
		t.Errorf("Status = %d, expected 300", summary.Status)
	}
	if len(summary.Probes) != 1 {
		t.Fatalf("Got %d probes in the summary, expected 1", len(summary.Probes))
	}
	if summary.Probes[0]["Name"] != "load" || summary.Probes[0]["Status"] != 300 {
		t.Errorf("Got %v, expected the load probe in status 300", summary.Probes[0])
	}
}

func TestHostAddOrUpdateProbe(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()

	// A new probe is attached to the host and updates the statuses
	probe := newTestProbe(nil, "load", 300)
	host.AddOrUpdateProbe(probe)

	if _, ok := host.Probes.Get("load"); !ok {
		t.Fatalf("The probe has not been added to the host")
	}
	if probe.GetHost() != host {
		t.Errorf("The probe parent host has not been set")
	}
	if host.Status != 300 {
		t.Errorf("Host status = %d, expected 300", host.Status)
	}
	if wigo.GlobalStatus != 300 {
		t.Errorf("Global status = %d, expected 300", wigo.GlobalStatus)
	}

	// Updating it with the same status raises no notification
	host.AddOrUpdateProbe(newTestProbe(host, "load", 300))
	if len(Channels.ChanCallbacks) != 0 {
		t.Errorf("A notification has been sent for an unchanged probe")
	}

	// Changing the status updates the host and notifies
	wigo.GetConfig().Notifications.OnProbeChange = true
	host.AddOrUpdateProbe(newTestProbe(host, "load", 100))

	if host.Status != 100 {
		t.Errorf("Host status = %d, expected 100", host.Status)
	}
	if len(Channels.ChanCallbacks) != 1 {
		t.Errorf("Got %d notifications, expected the status change to be notified", len(Channels.ChanCallbacks))
	}
}

func TestHostDeleteProbeByName(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()
	host.Probes.Set("load", newTestProbe(host, "load", 300))

	host.DeleteProbeByName("load")

	if _, ok := host.Probes.Get("load"); ok {
		t.Errorf("The probe has not been removed from the host")
	}

	// Deleting an unknown probe is a no-op
	host.DeleteProbeByName("does-not-exist")
}

func TestHostParentWigo(t *testing.T) {

	wigo := newTestRemoteWigo("uuid-1", "remote-1", "frontend")

	if wigo.GetLocalHost().GetParentWigo() != wigo {
		t.Errorf("The parent wigo of the local host is not the wigo itself")
	}
}
