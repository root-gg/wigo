package wigo

import (
	"strings"
	"testing"
)

func setupMetricsTest(t *testing.T) {
	t.Helper()

	setupTestWigo(t, "databases")

	flapping.Lock()
	flapping.transitions = make(map[string][]int64)
	flapping.since = make(map[string]int64)
	flapping.Unlock()
}

func metricLine(t *testing.T, output string, prefix string) string {
	t.Helper()

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}

	return ""
}

func TestMetricsExposeStatusAndSchedule(t *testing.T) {
	setupMetricsTest(t)

	result := NewProbeResult("check_load", 300, 0, "too high", "")
	result.Interval = 60
	result.Timestamp = 1700000000
	result.SetHost(LocalWigo.GetLocalHost())
	LocalWigo.GetLocalHost().Probes.Set("check_load", result)

	output := renderMetrics()

	want := map[string]string{
		"wigo_up":                                "wigo_up 1",
		"wigo_host_up{":                          `wigo_host_up{host="test-host",group="databases"} 1`,
		"wigo_probe_status{":                     `wigo_probe_status{host="test-host",group="databases",probe="check_load"} 300`,
		"wigo_probe_interval_seconds{":           `wigo_probe_interval_seconds{host="test-host",group="databases",probe="check_load"} 60`,
		"wigo_probe_last_run_timestamp_seconds{": `wigo_probe_last_run_timestamp_seconds{host="test-host",group="databases",probe="check_load"} 1700000000`,
	}

	for prefix, expected := range want {
		if got := metricLine(t, output, prefix); got != expected {
			t.Errorf("Got %q, expected %q", got, expected)
		}
	}

	// Every metric has to be declared, or a scraper has nothing to say about it
	for _, name := range []string{"wigo_up", "wigo_host_up", "wigo_probe_status"} {
		if !strings.Contains(output, "# HELP "+name+" ") || !strings.Contains(output, "# TYPE "+name+" gauge") {
			t.Errorf("%s is missing its HELP or TYPE", name)
		}
	}
}

// The half that makes it worth scraping : a load average was lost unless
// OpenTSDB was configured.
func TestMetricsExposeWhatTheProbeMeasured(t *testing.T) {
	setupMetricsTest(t)

	result := NewProbeResult("hardware_load_average", 100, 0, "fine", "")
	result.Metrics = []interface{}{
		map[string]interface{}{
			"Tags":  map[string]interface{}{"load": "load5"},
			"Value": float64(1.5),
		},
		map[string]interface{}{
			"Tags":  map[string]interface{}{"load": "load1"},
			"Value": float64(42),
		},
	}
	result.SetHost(LocalWigo.GetLocalHost())
	LocalWigo.GetLocalHost().Probes.Set("hardware_load_average", result)

	output := renderMetrics()

	expected := []string{
		`wigo_probe_metric{host="test-host",group="databases",probe="hardware_load_average",tag_load="load5"} 1.5`,
		`wigo_probe_metric{host="test-host",group="databases",probe="hardware_load_average",tag_load="load1"} 42`,
	}
	for _, line := range expected {
		if !strings.Contains(output, line) {
			t.Errorf("Missing %q in :\n%s", line, output)
		}
	}
}

// A metric without a numeric value is not a metric, and a probe that reports
// nonsense must not break the whole scrape.
func TestMetricsSkipWhatItCannotRead(t *testing.T) {
	setupMetricsTest(t)

	result := NewProbeResult("weird", 100, 0, "fine", "")
	result.Metrics = []interface{}{
		map[string]interface{}{"Tags": map[string]interface{}{"a": "b"}, "Value": "not a number"},
		map[string]interface{}{"Value": float64(7)},
		"not even a map",
	}
	result.SetHost(LocalWigo.GetLocalHost())
	LocalWigo.GetLocalHost().Probes.Set("weird", result)

	output := renderMetrics()

	if strings.Contains(output, "not a number") {
		t.Errorf("A non numeric value should have been skipped")
	}
	if !strings.Contains(output, `probe="weird"} 7`) {
		t.Errorf("The readable one should still be there :\n%s", output)
	}
	// And the rest of the scrape is intact
	if !strings.Contains(output, "wigo_up 1") {
		t.Errorf("The scrape should not have been derailed")
	}
}

// Tags come from probe authors, so anything can turn up in them.
func TestMetricLabelNamesAreSanitised(t *testing.T) {
	cases := map[string]string{
		"load":        "load",
		"mount point": "mount_point",
		"disk-usage":  "disk_usage",
		"1st":         "_st",
		"":            "_",
		"a1":          "a1",
	}

	for given, expected := range cases {
		if got := sanitiseLabelName(given); got != expected {
			t.Errorf("sanitiseLabelName(%q) = %q, expected %q", given, got, expected)
		}
	}
}

// A hostname with a quote in it would end the label and produce a scrape that
// does not parse.
func TestMetricLabelValuesAreEscaped(t *testing.T) {
	cases := map[string]string{
		`plain`:      `plain`,
		`with"quote`: `with\"quote`,
		`back\slash`: `back\\slash`,
		"new\nline":  `new\nline`,
	}

	for given, expected := range cases {
		if got := escapeLabelValue(given); got != expected {
			t.Errorf("escapeLabelValue(%q) = %q, expected %q", given, got, expected)
		}
	}
}

// Go map iteration is deliberately random. Label order changing between two
// scrapes makes any diff useless.
func TestMetricsAreStableBetweenScrapes(t *testing.T) {
	setupMetricsTest(t)

	result := NewProbeResult("check_disks", 100, 0, "fine", "")
	result.Metrics = []interface{}{
		map[string]interface{}{
			"Tags":  map[string]interface{}{"z": "1", "a": "2", "m": "3"},
			"Value": float64(1),
		},
	}
	result.SetHost(LocalWigo.GetLocalHost())
	LocalWigo.GetLocalHost().Probes.Set("check_disks", result)
	LocalWigo.GetLocalHost().Probes.Set("aaa", NewProbeResult("aaa", 100, 0, "fine", ""))

	first := renderMetrics()
	for i := 0; i < 5; i++ {
		if renderMetrics() != first {
			t.Fatalf("Two scrapes of the same state must be identical")
		}
	}

	// Probes in name order, so a diff between two scrapes is readable
	if strings.Index(first, `probe="aaa"`) > strings.Index(first, `probe="check_disks"`) {
		t.Errorf("Probes should be exposed in name order")
	}
}

// Alerting on your own alerting : an ack nobody lifted and a probe flapping for
// a week are both monitoring that quietly stopped reaching anyone.
func TestMetricsExposeWhatIsBeingHeldBack(t *testing.T) {
	setupMetricsTest(t)
	LocalWigo.config.Notifications.FlapDetection = true
	LocalWigo.config.Notifications.FlapThreshold = 2
	LocalWigo.config.Notifications.FlapWindow = 3600

	result := NewProbeResult("check_load", 300, 0, "too high", "")
	result.SetHost(LocalWigo.GetLocalHost())
	LocalWigo.GetLocalHost().Probes.Set("check_load", result)

	RecordStatusChange("test-host", "check_load")
	RecordStatusChange("test-host", "check_load")
	ackOn(t, "test-host", "check_load", 300)

	output := renderMetrics()

	if !strings.Contains(output, `wigo_probe_flapping{host="test-host",group="databases",probe="check_load"} 1`) {
		t.Errorf("The flapping probe should be exposed :\n%s", output)
	}
	if !strings.Contains(output, `wigo_notifications_suppressed{scope="host",target="test-host",probe="check_load",kind="ack"} 1`) {
		t.Errorf("The ack should be exposed :\n%s", output)
	}
}

// A steady probe has to say so rather than be absent : a missing series cannot
// be alerted on, an explicit zero can.
func TestASteadyProbeReportsZeroRatherThanNothing(t *testing.T) {
	setupMetricsTest(t)

	result := NewProbeResult("check_load", 100, 0, "fine", "")
	result.SetHost(LocalWigo.GetLocalHost())
	LocalWigo.GetLocalHost().Probes.Set("check_load", result)

	output := renderMetrics()

	if !strings.Contains(output, `wigo_probe_flapping{host="test-host",group="databases",probe="check_load"} 0`) {
		t.Errorf("A steady probe should report zero :\n%s", output)
	}
}

func TestFormatFloat(t *testing.T) {
	cases := map[float64]string{
		1.5:     "1.5",
		42:      "42",
		0:       "0",
		1000000: "1000000",
		0.125:   "0.125",
		-3.25:   "-3.25",
	}

	for given, expected := range cases {
		if got := formatFloat(given); got != expected {
			t.Errorf("formatFloat(%v) = %q, expected %q", given, got, expected)
		}
	}
}
