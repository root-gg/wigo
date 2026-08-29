package wigo

import (
	"testing"
	"time"
)

func setupHistoryTest(t *testing.T) {
	t.Helper()

	setupTestWigo(t, "databases")
	LocalWigo.config.Global.MetricsRetentionDays = 7
}

func recordAt(t *testing.T, probe string, at int64, metrics ...interface{}) {
	t.Helper()

	result := NewProbeResult(probe, 100, 0, "fine", "")
	result.Timestamp = at
	result.Metrics = metrics
	result.SetHost(LocalWigo.GetLocalHost())

	RecordProbeMetrics(result)
}

func metric(value float64, tags map[string]interface{}) interface{} {
	entry := map[string]interface{}{"Value": value}
	if tags != nil {
		entry["Tags"] = tags
	}
	return entry
}

// The point of the whole thing : the answer to "was it already climbing an hour
// ago" used to be no.
func TestWhatAProbeMeasuredIsKept(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	for i := int64(0); i < 5; i++ {
		recordAt(t, "load", now-300+i*60, metric(float64(i), map[string]interface{}{"load": "load1"}))
	}

	series, err := ProbeMetrics("load", now-3600, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 1 {
		t.Fatalf("Got %+v, expected one series", series)
	}
	if series[0].Tags["load"] != "load1" {
		t.Errorf("Got %+v, expected the probe's own tags", series[0].Tags)
	}
	if len(series[0].Points) != 5 {
		t.Errorf("Got %d points, expected 5", len(series[0].Points))
	}
}

// Two sets of tags are two lines on a graph, not one.
func TestTagsSeparateTheSeries(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	recordAt(t, "load", now-60,
		metric(1, map[string]interface{}{"load": "load1"}),
		metric(5, map[string]interface{}{"load": "load5"}))

	series, err := ProbeMetrics("load", now-3600, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 2 {
		t.Fatalf("Got %d series, expected one per tag set", len(series))
	}
	// Sorted, so two reads of the same window draw the same graph
	if series[0].Tags["load"] != "load1" || series[1].Tags["load"] != "load5" {
		t.Errorf("Got %+v and %+v, expected them in tag order", series[0].Tags, series[1].Tags)
	}
}

// Go map iteration is deliberately random. Unsorted, the same series would be
// split across as many rows as it has moods.
func TestTheSameTagsAlwaysProduceTheSameKey(t *testing.T) {
	first := encodeMetricTags(map[string]interface{}{"b": "2", "a": "1", "c": "3"})

	for i := 0; i < 20; i++ {
		if again := encodeMetricTags(map[string]interface{}{"c": "3", "a": "1", "b": "2"}); again != first {
			t.Fatalf("Got %q then %q for the same tags", first, again)
		}
	}
	if first != "a=1,b=2,c=3" {
		t.Errorf("Got %q", first)
	}

	if encodeMetricTags(nil) != "" || encodeMetricTags(map[string]interface{}{}) != "" {
		t.Errorf("No tags should encode to nothing")
	}
}

func TestDecodeMetricTags(t *testing.T) {
	tags := decodeMetricTags("a=1,b=2")
	if len(tags) != 2 || tags["a"] != "1" || tags["b"] != "2" {
		t.Errorf("Got %+v", tags)
	}

	if len(decodeMetricTags("")) != 0 {
		t.Errorf("Nothing should decode to no tags")
	}
}

// A week at one point a minute is ten thousand points, which no browser should
// be asked to draw. The bucket carries what the average hid.
func TestPointsAreBucketedAndKeepTheirRange(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	// Ten minutes of values, one a minute, with one spike
	for i := int64(0); i < 10; i++ {
		value := float64(1)
		if i == 4 {
			value = 100
		}
		recordAt(t, "load", now-600+i*60, metric(value, nil))
	}

	series, err := ProbeMetrics("load", now-600, now, 2)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 1 {
		t.Fatalf("Got %+v", series)
	}
	if len(series[0].Points) > 3 {
		t.Errorf("Got %d points, expected them bucketed", len(series[0].Points))
	}

	// The spike has to survive being averaged, or a graph quietly erases the
	// thing that woke somebody up
	highest := float64(0)
	for _, point := range series[0].Points {
		if point.Max > highest {
			highest = point.Max
		}
	}
	if highest != 100 {
		t.Errorf("Got a maximum of %v, expected the spike to still be visible", highest)
	}
}

func TestOnlyTheWindowAskedForIsRead(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	recordAt(t, "load", now-7200, metric(1, nil))
	recordAt(t, "load", now-60, metric(2, nil))

	series, err := ProbeMetrics("load", now-3600, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 {
		t.Fatalf("Got %+v, expected only the recent one", series)
	}
	if series[0].Points[0].Value != 2 {
		t.Errorf("Got %v, expected the value inside the window", series[0].Points[0].Value)
	}
}

// A store that never forgets is a disk that fills up.
func TestAgedMetricsAreDropped(t *testing.T) {
	setupHistoryTest(t)
	LocalWigo.config.Global.MetricsRetentionDays = 1

	now := time.Now().Unix()
	recordAt(t, "load", now-86400*3, metric(1, nil))
	recordAt(t, "load", now-60, metric(2, nil))

	dropAgedMetrics()

	series, err := ProbeMetrics("load", now-86400*7, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 {
		t.Fatalf("Got %+v, expected the old point to be gone", series)
	}
	if series[0].Points[0].Value != 2 {
		t.Errorf("Got %v, expected the recent one to have been kept", series[0].Points[0].Value)
	}
}

// Nothing about the monitoring depends on this, so turning it off has to be a
// real option rather than a slow leak.
func TestNothingIsKeptWhenRetentionIsZero(t *testing.T) {
	setupHistoryTest(t)
	LocalWigo.config.Global.MetricsRetentionDays = 0

	recordAt(t, "load", time.Now().Unix(), metric(1, nil))

	series, err := ProbeMetrics("load", time.Now().Unix()-3600, time.Now().Unix(), 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 0 {
		t.Errorf("Got %+v, expected nothing to have been kept", series)
	}
}

// A probe reporting something that is not a number must not derail the write,
// nor take the rest of the result down with it.
func TestWhatIsNotANumberIsSkipped(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	recordAt(t, "weird", now-60,
		map[string]interface{}{"Value": "not a number"},
		"not even a map",
		metric(7, nil))

	series, err := ProbeMetrics("weird", now-3600, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 7 {
		t.Errorf("Got %+v, expected only the readable one", series)
	}
}

func TestProbeMetricsRefusesWhatItCannotAnswer(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()

	if _, err := ProbeMetrics("../../etc/passwd", now-3600, now, 300); err == nil {
		t.Errorf("A traversing probe name should be refused")
	}
	if _, err := ProbeMetrics("load", now, now-3600, 300); err == nil {
		t.Errorf("A window that ends before it starts should be refused")
	}
}
