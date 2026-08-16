package wigo

import (
	"encoding/json"
	"testing"
)

// Remote wigos and probes are exchanged as plain json objects but stored in
// concurrent maps, so they need their own unmarshaller.
func TestConcurrentMapWigosUnmarshalJSON(t *testing.T) {

	wigos := NewConcurrentMapWigos()

	payload := `{"uuid-1":{"Uuid":"uuid-1","Hostname":"remote-1"},"uuid-2":{"Uuid":"uuid-2","Hostname":"remote-2"}}`
	if err := json.Unmarshal([]byte(payload), wigos); err != nil {
		t.Fatalf("Unmarshal returned an error : %s", err)
	}

	if wigos.Count() != 2 {
		t.Fatalf("Got %d wigos, expected 2", wigos.Count())
	}

	tmp, ok := wigos.Get("uuid-1")
	if !ok {
		t.Fatalf("uuid-1 is missing from the map")
	}
	if hostname := tmp.(*Wigo).GetHostname(); hostname != "remote-1" {
		t.Errorf("Hostname = %s, expected remote-1", hostname)
	}
}

func TestConcurrentMapProbesUnmarshalJSON(t *testing.T) {

	probes := NewConcurrentMapProbes()

	payload := `{"load":{"Name":"load","Status":300,"Message":"load too high"}}`
	if err := json.Unmarshal([]byte(payload), probes); err != nil {
		t.Fatalf("Unmarshal returned an error : %s", err)
	}

	tmp, ok := probes.Get("load")
	if !ok {
		t.Fatalf("load is missing from the map")
	}

	probe := tmp.(*ProbeResult)
	if probe.Status != 300 || probe.Message != "load too high" {
		t.Errorf("Got %+v, expected the load probe in status 300", probe)
	}
}

// Both unmarshallers swallow malformed payloads and leave the map untouched
// instead of propagating the error.
func TestConcurrentMapUnmarshalInvalidJSON(t *testing.T) {

	wigos := NewConcurrentMapWigos()
	if err := json.Unmarshal([]byte(`["not","an","object"]`), wigos); err != nil {
		t.Errorf("Unmarshal returned an error : %s", err)
	}
	if wigos.Count() != 0 {
		t.Errorf("Got %d wigos, expected the map to stay empty", wigos.Count())
	}

	probes := NewConcurrentMapProbes()
	if err := json.Unmarshal([]byte(`["not","an","object"]`), probes); err != nil {
		t.Errorf("Unmarshal returned an error : %s", err)
	}
	if probes.Count() != 0 {
		t.Errorf("Got %d probes, expected the map to stay empty", probes.Count())
	}
}
