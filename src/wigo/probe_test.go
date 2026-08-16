package wigo

import (
	"strings"
	"testing"
)

func TestNewProbeResult(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	probe := NewProbeResult("load", 300, 1, "load too high", "detail")

	if probe.Name != "load" || probe.Status != 300 || probe.ExitCode != 1 {
		t.Errorf("Got %+v, expected the load probe in status 300 with exit code 1", probe)
	}
	if probe.Message != "load too high" || probe.Detail != "detail" {
		t.Errorf("Got message %q and detail %v", probe.Message, probe.Detail)
	}
	if probe.GetHost() != wigo.GetLocalHost() {
		t.Errorf("The probe is not attached to the local host")
	}
	if probe.ProbeDate == "" || probe.Timestamp == 0 {
		t.Errorf("The probe execution date has not been set")
	}
}

func TestNewProbeResultFromJson(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	payload := []byte(`{"Status":250,"Message":"disk almost full","Detail":{"used":"91%"},"Version":"1.0"}`)
	probe := NewProbeResultFromJson("disk", payload)

	if probe.Name != "disk" {
		t.Errorf("Name = %s, expected the name given to the constructor", probe.Name)
	}
	if probe.Status != 250 || probe.Message != "disk almost full" || probe.Version != "1.0" {
		t.Errorf("Got %+v, expected the payload values", probe)
	}
	if probe.Detail == nil {
		t.Errorf("The probe detail has been dropped")
	}
	// The exit code is reset, it is only meaningful for probes that crashed
	if probe.ExitCode != 0 {
		t.Errorf("ExitCode = %d, expected 0", probe.ExitCode)
	}
	if probe.GetHost() != wigo.GetLocalHost() {
		t.Errorf("The probe is not attached to the local host")
	}
}

// A probe printing garbage must not crash the collector, it just yields an
// empty result.
func TestNewProbeResultFromInvalidJson(t *testing.T) {

	setupTestWigo(t, "databases")

	probe := NewProbeResultFromJson("broken", []byte("this is not json"))

	if probe == nil {
		t.Fatalf("Expected a probe result even for an invalid payload")
	}
	if probe.Name != "broken" {
		t.Errorf("Name = %s, expected broken", probe.Name)
	}
	if probe.Status != 0 || probe.Message != "" {
		t.Errorf("Got %+v, expected an empty result", probe)
	}
}

func TestProbeResultSummary(t *testing.T) {

	setupTestWigo(t, "databases")

	probe := NewProbeResult("load", 300, 0, "load too high", "")
	probe.Detail = map[string]interface{}{"load1": 12.5}

	summary := probe.Summary()

	for _, expected := range []string{"load", "300", "load too high", "load1"} {
		if !strings.Contains(summary, expected) {
			t.Errorf("The summary does not mention %q :\n%s", expected, summary)
		}
	}
}

func TestProbeResultSummaryWithoutDetail(t *testing.T) {

	setupTestWigo(t, "databases")

	probe := NewProbeResult("load", 100, 0, "everything is fine", "")
	probe.Detail = nil

	if summary := probe.Summary(); strings.Contains(summary, "Detail") {
		t.Errorf("The summary should not have a detail section :\n%s", summary)
	}
}

// Metrics are only pushed when OpenTSDB is enabled, and the call must stay
// harmless otherwise.
func TestProbeResultGraphMetricsWithoutOpenTSDB(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	wigo.GetConfig().OpenTSDB.Enabled = false

	probe := NewProbeResult("load", 100, 0, "ok", "")
	probe.Metrics = []interface{}{map[string]interface{}{"Value": 1.0}}

	probe.GraphMetrics()
}
