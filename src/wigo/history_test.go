package wigo

import (
	"testing"
	"time"
)

func setupHistoryTest(t *testing.T) {
	t.Helper()

	setupTestWigo(t, "databases")
	LocalWigo.config.Global.MetricsRetentionDays = 7

	// What was already pushed is remembered across a whole process, so one test
	// would otherwise decide what the next one is allowed to record.
	lastPushedMetric.Lock()
	lastPushedMetric.at = make(map[string]int64)
	lastPushedMetric.Unlock()
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

// A host that pushes cannot be asked anything, so the moment its measurements
// arrive is the only moment they can be written down. Half a fleet had no
// graphs at all before this, and the screen answered a 501 saying why.
func TestAPushingClientsMeasurementsAreKept(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	client := newTestRemoteWigo("uuid-push", "behind-nat", "frontend")

	// Oldest first, which is the order a client actually pushes them in
	for i := int64(4); i >= 0; i-- {
		probe := newTestProbe(client.LocalHost, "load", 100)
		probe.Timestamp = now - i*60
		probe.Metrics = []interface{}{metric(float64(i), map[string]interface{}{"metric": "load5"})}
		client.LocalHost.Probes.Set("load", probe)

		RecordPushedMetrics(client)
	}

	series, err := ProbeMetricsOf("behind-nat", "load", now-3600, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 1 {
		t.Fatalf("Got %+v, expected one series", series)
	}
	if len(series[0].Points) != 5 {
		t.Errorf("Got %d points, expected the five that were pushed", len(series[0].Points))
	}
}

// One host's measurements must not answer for another's : the graph would show
// a machine that was never asked about.
func TestOneHostsMeasurementsDoNotAnswerForAnother(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	recordAt(t, "load", now-60, metric(1, nil))

	client := newTestRemoteWigo("uuid-push", "behind-nat", "frontend")
	probe := newTestProbe(client.LocalHost, "load", 100)
	probe.Timestamp = now - 60
	probe.Metrics = []interface{}{metric(42, nil)}
	client.LocalHost.Probes.Set("load", probe)
	RecordPushedMetrics(client)

	local, err := ProbeMetricsOf(LocalWigo.GetHostname(), "load", now-3600, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(local) != 1 || len(local[0].Points) != 1 || local[0].Points[0].Value != 1 {
		t.Errorf("Got %+v, expected only what was measured here", local)
	}

	pushed, err := ProbeMetricsOf("behind-nat", "load", now-3600, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(pushed) != 1 || len(pushed[0].Points) != 1 || pushed[0].Points[0].Value != 42 {
		t.Errorf("Got %+v, expected only what the client sent", pushed)
	}

	// And a host nobody recorded anything for has nothing, rather than
	// everything.
	if other, err := ProbeMetricsOf("someone-else", "load", now-3600, now, 300); err != nil || len(other) != 0 {
		t.Errorf("Got %+v (%v), expected nothing", other, err)
	}
}

// A database written before metrics carried a host has to gain the column, and
// what is already in it was measured here : nothing else could have been.
func TestAnOlderDatabaseGainsTheHostColumn(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()

	// Put it back the way it was, rows included
	if _, err := LocalWigo.sqlLiteConn.Exec(`DROP TABLE metrics;`); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if _, err := LocalWigo.sqlLiteConn.Exec(`
		CREATE TABLE metrics (
			id integer not null primary key,
			probe text not null,
			tags text not null,
			value real not null,
			at int not null
		) ;`); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if _, err := LocalWigo.sqlLiteConn.Exec(
		`INSERT INTO metrics(probe,tags,value,at) VALUES('load','',7,?);`, now-60); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if err := migrateMetricsHost(LocalWigo.sqlLiteConn, LocalWigo.GetHostname()); err != nil {
		t.Fatalf("The migration failed : %s", err)
	}

	series, err := ProbeMetricsOf(LocalWigo.GetHostname(), "load", now-3600, now, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 7 {
		t.Errorf("Got %+v, expected the old row attributed to this host", series)
	}

	// Twice is not an error : it runs at every startup.
	if err := migrateMetricsHost(LocalWigo.sqlLiteConn, LocalWigo.GetHostname()); err != nil {
		t.Errorf("Running it again failed : %s", err)
	}
}

// A client pushes far more often than its probes run, and each push carries the
// same result again. Writing it every time stored one measurement a dozen times
// -- the master's database growing for nothing, which is the cost the whole
// choice was weighed against.
func TestTheSameMeasurementPushedTwiceIsWrittenOnce(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	client := newTestRemoteWigo("uuid-push", "behind-nat", "frontend")

	probe := newTestProbe(client.LocalHost, "load", 100)
	probe.Timestamp = now - 60
	probe.Metrics = []interface{}{metric(3, nil)}
	client.LocalHost.Probes.Set("load", probe)

	// Twelve pushes between two runs of the probe, which is what a five second
	// interval against a minute one gives.
	for i := 0; i < 12; i++ {
		RecordPushedMetrics(client)
	}

	if rows := countMetricRows(t, "behind-nat", "load"); rows != 1 {
		t.Errorf("Got %d rows, expected the measurement to be written once", rows)
	}

	// And the next run is written, since it is a different measurement
	next := newTestProbe(client.LocalHost, "load", 100)
	next.Timestamp = now
	next.Metrics = []interface{}{metric(4, nil)}
	client.LocalHost.Probes.Set("load", next)
	RecordPushedMetrics(client)

	if rows := countMetricRows(t, "behind-nat", "load"); rows != 2 {
		t.Errorf("Got %d rows, expected the newer measurement to be kept too", rows)
	}

	series, err := ProbeMetricsOf("behind-nat", "load", now-3600, now+60, 300)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if len(series) != 1 || len(series[0].Points) != 2 {
		t.Errorf("Got %+v, expected the two measurements", series)
	}
}

// A result older than the last one seen is not a new measurement : it is the
// same push arriving out of order, or a client whose clock went back.
func TestAnOlderMeasurementDoesNotReopenTheDoor(t *testing.T) {
	setupHistoryTest(t)

	now := time.Now().Unix()
	client := newTestRemoteWigo("uuid-push", "behind-nat", "frontend")

	for _, at := range []int64{now, now - 120} {
		probe := newTestProbe(client.LocalHost, "load", 100)
		probe.Timestamp = at
		probe.Metrics = []interface{}{metric(1, nil)}
		client.LocalHost.Probes.Set("load", probe)
		RecordPushedMetrics(client)
	}

	if rows := countMetricRows(t, "behind-nat", "load"); rows != 1 {
		t.Errorf("Got %d rows, expected only the newest to have been kept", rows)
	}
}

func countMetricRows(t *testing.T, hostname string, probe string) int {
	t.Helper()

	var count int
	row := LocalWigo.sqlLiteConn.QueryRow(
		`SELECT count(*) FROM metrics WHERE host = ? AND probe = ?;`, hostname, probe)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	return count
}
