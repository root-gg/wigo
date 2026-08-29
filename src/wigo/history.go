package wigo

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Keeping what the probes measured.
//
// A probe reports a load average, a disk usage, a queue length. Until now all
// of it was thrown away the moment the next run replaced it, unless an OpenTSDB
// was configured -- which means the answer to "was it already climbing an hour
// ago" was no, and the only way to get one was to deploy a time series database
// next to a monitoring tool that already has one open.
//
// So it is written down, in the sqlite that is already there, bounded by a
// retention. Nothing about the monitoring depends on it : losing this table
// loses history and nothing else.
//
// Each wigo keeps its own, and only its own. A master reads a remote's history
// through that remote's api, the same way it reads its schedule. Storing the
// fleet's series on the master as well would write everything twice and make
// the master's database grow with the size of the fleet, which is exactly the
// thing that pushes people towards a separate stack.

const defaultMetricsRetentionDays = 7

const createMetricsTable = `
    CREATE TABLE IF NOT EXISTS metrics (
        id integer not null primary key,
        probe text not null,
        tags text not null,
        value real not null,
        at int not null
    ) ;
    CREATE INDEX IF NOT EXISTS metrics_lookup ON metrics(probe, tags, at) ;
    `

// MetricPoint is one measurement, or one bucket of them.
type MetricPoint struct {
	At int64

	// The average over the bucket, and what it hid. A graph drawn on averages
	// alone quietly erases the spike that woke somebody up.
	Value float64
	Min   float64
	Max   float64
}

// MetricSeries is one measured thing over time.
type MetricSeries struct {
	Probe string

	// The probe's own tags, as it reported them.
	Tags map[string]string

	Points []MetricPoint
}

func metricsRetentionDays() int {
	days := GetLocalWigo().GetConfig().Global.MetricsRetentionDays
	if days < 0 {
		return 0
	}

	return days
}

// RecordProbeMetrics writes down what a probe just measured.
//
// Called for this host's probes only. Failing is logged and never propagated :
// the result is already computed, displayed and notified about, and losing a
// point of history is not a reason to fail any of that.
func RecordProbeMetrics(result *ProbeResult) {
	if result == nil || metricsRetentionDays() == 0 {
		return
	}
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return
	}

	values, ok := result.Metrics.([]interface{})
	if !ok || len(values) == 0 {
		return
	}

	at := result.Timestamp
	if at == 0 {
		at = time.Now().Unix()
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	// One transaction for the whole result : a probe reporting five metrics is
	// one thing that happened, and five fsyncs a minute per probe adds up.
	transaction, err := LocalWigo.sqlLiteConn.Begin()
	if err != nil {
		log.Printf("Unable to record the metrics of probe %s : %s", result.Name, err)
		return
	}

	statement, err := transaction.Prepare(
		`INSERT INTO metrics(probe,tags,value,at) VALUES(?,?,?,?);`)
	if err != nil {
		_ = transaction.Rollback()
		log.Printf("Unable to record the metrics of probe %s : %s", result.Name, err)
		return
	}
	defer statement.Close()

	for _, entry := range values {
		metric, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}

		value, ok := metric["Value"].(float64)
		if !ok {
			continue
		}

		if _, err := statement.Exec(result.Name, encodeMetricTags(metric["Tags"]), value, at); err != nil {
			_ = transaction.Rollback()
			log.Printf("Unable to record the metrics of probe %s : %s", result.Name, err)
			return
		}
	}

	if err := transaction.Commit(); err != nil {
		log.Printf("Unable to record the metrics of probe %s : %s", result.Name, err)
	}
}

// encodeMetricTags turns a probe's tags into the one string that identifies a
// series.
//
// Sorted, so the same set of tags always produces the same key : unsorted, the
// same series would be split in as many rows as go's map iteration has moods.
func encodeMetricTags(raw interface{}) string {
	values, ok := raw.(map[string]interface{})
	if !ok || len(values) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(values))
	for key, value := range values {
		text, ok := value.(string)
		if !ok {
			text = fmt.Sprintf("%v", value)
		}
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, text))
	}

	sort.Strings(pairs)

	return strings.Join(pairs, ",")
}

func decodeMetricTags(encoded string) map[string]string {
	tags := make(map[string]string)
	if encoded == "" {
		return tags
	}

	for _, pair := range strings.Split(encoded, ",") {
		key, value, found := strings.Cut(pair, "=")
		if found {
			tags[key] = value
		}
	}

	return tags
}

// ProbeMetrics reads the history of one probe, bucketed to at most that many
// points per series.
//
// Bucketing is what makes this usable : a week at one point a minute is ten
// thousand points per series, which no browser should be asked to draw and no
// eye can read. Each bucket carries its average and the range it covers, so the
// spike that woke somebody up is still visible after being averaged.
func ProbeMetrics(probe string, since int64, until int64, points int) ([]MetricSeries, error) {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return nil, fmt.Errorf("no database to read metrics from")
	}
	if !IsValidProbeName(probe) {
		return nil, fmt.Errorf("invalid probe name %q", probe)
	}

	if until <= 0 {
		until = time.Now().Unix()
	}
	if since <= 0 || since >= until {
		return nil, fmt.Errorf("the window has to start before it ends")
	}
	if points < 1 {
		points = 1
	}

	bucket := (until - since) / int64(points)
	if bucket < 1 {
		bucket = 1
	}

	LocalWigo.sqlLiteLock.Lock()
	rows, err := LocalWigo.sqlLiteConn.Query(
		`SELECT tags, (at / ?) * ? AS bucket, avg(value), min(value), max(value)
		 FROM metrics
		 WHERE probe = ? AND at >= ? AND at <= ?
		 GROUP BY tags, bucket
		 ORDER BY tags, bucket;`,
		bucket, bucket, probe, since, until)
	if err != nil {
		LocalWigo.sqlLiteLock.Unlock()
		return nil, err
	}

	byTags := make(map[string]*MetricSeries)
	order := make([]string, 0)

	for rows.Next() {
		var tags string
		var point MetricPoint

		if err := rows.Scan(&tags, &point.At, &point.Value, &point.Min, &point.Max); err != nil {
			continue
		}

		series, known := byTags[tags]
		if !known {
			series = &MetricSeries{
				Probe:  probe,
				Tags:   decodeMetricTags(tags),
				Points: make([]MetricPoint, 0),
			}
			byTags[tags] = series
			order = append(order, tags)
		}

		series.Points = append(series.Points, point)
	}

	err = rows.Err()
	rows.Close()
	LocalWigo.sqlLiteLock.Unlock()

	if err != nil {
		return nil, err
	}

	// In tag order, so two reads of the same window draw the same graph
	sort.Strings(order)

	all := make([]MetricSeries, 0, len(order))
	for _, tags := range order {
		all = append(all, *byTags[tags])
	}

	return all, nil
}

// StartMetricsRetention drops what has aged out.
//
// Hourly rather than continuously : this is a bounded store, not a precise one,
// and an hour of extra rows costs nothing next to waking the disk every minute.
func StartMetricsRetention() {
	days := metricsRetentionDays()
	if days == 0 {
		log.Printf("Metrics : nothing is kept, set MetricsRetentionDays to keep a history")
		return
	}

	log.Printf("Metrics : keeping %d days of what the probes measure", days)

	go func() {
		for {
			dropAgedMetrics()
			time.Sleep(time.Hour)
		}
	}()
}

func dropAgedMetrics() {
	days := metricsRetentionDays()
	if days == 0 || LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return
	}

	oldest := time.Now().Unix() - int64(days)*86400

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	if _, err := LocalWigo.sqlLiteConn.Exec(`DELETE FROM metrics WHERE at < ?;`, oldest); err != nil {
		log.Printf("Fail to drop the aged metrics : %s", err)
	}
}

// ProbeHistory is what the metrics endpoint answers.
type ProbeHistory struct {
	Hostname string
	Probe    string

	// The window actually read, which is not always the one asked for : a
	// graph has to be able to say what it is showing.
	Since int64
	Until int64

	// How many days this host keeps. Zero means it keeps none, which is why an
	// empty answer is not the same as a probe that measured nothing.
	RetentionDays int

	Series []MetricSeries
}

// HttpProbeMetricsHandler answers the history of one probe of this host.
func HttpProbeMetricsHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	probe := r.PathValue("probe")
	if probe == "" {
		return 404, "No probe name set in url"
	}

	since, until, points, err := parseMetricsWindow(r)
	if err != nil {
		return 400, err.Error()
	}

	series, err := ProbeMetrics(probe, since, until, points)
	if err != nil {
		return 400, err.Error()
	}

	body, err := json.Marshal(ProbeHistory{
		Hostname:      GetLocalWigo().GetHostname(),
		Probe:         probe,
		Since:         since,
		Until:         until,
		RetentionDays: metricsRetentionDays(),
		Series:        series,
	})
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the metrics : %s", err)
	}

	return 200, string(body)
}

// HttpHostProbeMetricsHandler answers for any host of the tree.
//
// A remote's history lives on that remote, so this forwards rather than looking
// in a local store that would only ever hold this host's own measurements.
func HttpHostProbeMetricsHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	hostname := r.PathValue("hostname")
	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	if hostname == GetLocalWigo().GetHostname() {
		return HttpProbeMetricsHandler(w, r)
	}

	if !IsValidProbeName(r.PathValue("probe")) {
		return 400, fmt.Sprintf("invalid probe name %q", r.PathValue("probe"))
	}

	// A host that pushes to us cannot be asked : we hold its results, not its
	// history, and it is the one keeping that.
	if remote := GetLocalWigo().FindRemoteWigoByHostname(hostname); remote != nil {
		if _, polled := remoteEndpointFor(remote.Uuid); !polled {
			return 501, fmt.Sprintf("%s cannot be asked for its history from here : it pushes to this host "+
				"rather than being polled, and each wigo keeps its own measurements.", hostname)
		}
	}

	path := "/api/probes/" + url.PathEscape(r.PathValue("probe")) + "/metrics"

	return forwardToRemote(hostname, "GET", path, r.URL.Query())
}

// parseMetricsWindow reads what to look at, with defaults that draw something
// useful for somebody who just asked for a graph.
func parseMetricsWindow(r *http.Request) (int64, int64, int, error) {
	now := time.Now().Unix()

	since := now - 3600
	if raw := r.URL.Query().Get("since"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid since %q, expected a unix timestamp", raw)
		}
		since = value
	}

	until := now
	if raw := r.URL.Query().Get("until"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid until %q, expected a unix timestamp", raw)
		}
		until = value
	}

	// Enough to draw a line, few enough that a browser can. Asking for more is
	// asking for points no screen has pixels for.
	points := 300
	if raw := r.URL.Query().Get("points"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxMetricsPoints {
			return 0, 0, 0, fmt.Errorf("invalid points %q, expected between 1 and %d", raw, maxMetricsPoints)
		}
		points = value
	}

	return since, until, points, nil
}

// A cap, because the number of buckets is the number of rows the answer holds
// and a caller should not be able to ask for a million of them.
const maxMetricsPoints = 2000
