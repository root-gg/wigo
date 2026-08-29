package wigo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
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
// A wigo keeps its own, and a master reads a polled remote's through that
// remote's api, the same way it reads its schedule. Storing the whole fleet's
// series on the master would write everything twice and make its database grow
// with the size of the fleet, which is exactly the thing that pushes people
// towards a separate stack.
//
// With one exception, and it is not a softening of that rule but the only place
// it cannot hold : a host that pushes cannot be asked anything, it sits behind
// a NAT. Its measurements arrive here with every push and used to be dropped,
// so half a fleet had no graphs at all and the screen said so in a 501. Those
// are kept, under the name of the host they came from. The growth is bounded by
// the number of pushing clients rather than by the fleet, and those clients are
// precisely the ones with no other way of being read.

const defaultMetricsRetentionDays = 7

// Rows carry the host they were measured on. Local ones say so too rather than
// leaving it empty : a column that means "here" for some rows and names a host
// for others is one every later query has to remember to special case.
const createMetricsTable = `
    CREATE TABLE IF NOT EXISTS metrics (
        id integer not null primary key,
        host text not null default '',
        probe text not null,
        tags text not null,
        value real not null,
        at int not null
    ) ;
    CREATE INDEX IF NOT EXISTS metrics_lookup ON metrics(host, probe, tags, at) ;
    `

// migrateMetricsHost adds the column to a table written before it existed, and
// attributes what is already in there to this host -- which is what it was,
// since nothing else could be recorded then.
func migrateMetricsHost(db *sql.DB, hostname string) error {
	rows, err := db.Query(`PRAGMA table_info(metrics);`)
	if err != nil {
		return err
	}

	found := false
	for rows.Next() {
		var index int
		var name, kind string
		var notNull, primaryKey int
		var fallback interface{}

		if err := rows.Scan(&index, &name, &kind, &notNull, &fallback, &primaryKey); err != nil {
			continue
		}
		if name == "host" {
			found = true
		}
	}
	rows.Close()

	if found {
		return nil
	}

	if _, err := db.Exec(`ALTER TABLE metrics ADD COLUMN host text not null default '';`); err != nil {
		return err
	}

	// Everything already there was measured here
	if _, err := db.Exec(`UPDATE metrics SET host = ? WHERE host = '';`, hostname); err != nil {
		return err
	}

	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS metrics_lookup ON metrics(host, probe, tags, at) ;`)

	return err
}

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
	RecordProbeMetricsOf(GetLocalWigo().GetHostname(), result)
}

// RecordProbeMetricsOf writes down what a probe measured on a named host.
//
// Used for this host, and for the ones that push to it : their measurements
// arrive with every push and there is nowhere else they could be read from.
func RecordProbeMetricsOf(hostname string, result *ProbeResult) {
	if hostname == "" || result == nil || metricsRetentionDays() == 0 {
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
		`INSERT INTO metrics(host,probe,tags,value,at) VALUES(?,?,?,?,?);`)
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

		if _, err := statement.Exec(hostname, result.Name, encodeMetricTags(metric["Tags"]), value, at); err != nil {
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
	return ProbeMetricsOf(GetLocalWigo().GetHostname(), probe, since, until, points)
}

// ProbeMetricsOf is the same question asked about a named host, which is how a
// master answers for a client that pushes to it.
func ProbeMetricsOf(hostname string, probe string, since int64, until int64, points int) ([]MetricSeries, error) {
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
		 WHERE host = ? AND probe = ? AND at >= ? AND at <= ?
		 GROUP BY tags, bucket
		 ORDER BY tags, bucket;`,
		bucket, bucket, hostname, probe, since, until)
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
	return localMetricsFor(GetLocalWigo().GetHostname(), r)
}

// localMetricsFor answers from this database, for whichever host the series
// were recorded under : this one, or a client that pushes to it.
func localMetricsFor(hostname string, r *http.Request) (int, string) {

	probe := r.PathValue("probe")
	if probe == "" {
		return 404, "No probe name set in url"
	}

	since, until, points, err := parseMetricsWindow(r)
	if err != nil {
		return 400, err.Error()
	}

	series, err := ProbeMetricsOf(hostname, probe, since, until, points)
	if err != nil {
		return 400, err.Error()
	}

	body, err := json.Marshal(ProbeHistory{
		Hostname:      hostname,
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

	// A host that pushes to us cannot be asked, so we answer from what it sent.
	// Nothing is relayed for it : this master is where its history lives.
	if remote := GetLocalWigo().FindRemoteWigoByHostname(hostname); remote != nil {
		if _, polled := remoteEndpointFor(remote.Uuid); !polled {
			return localMetricsFor(hostname, r)
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

// The last measurement written down for a host and a probe.
//
// A client pushes far more often than its probes run -- every five seconds
// against every minute is a normal pairing -- and each push carries the same
// result again. Writing it every time stores one measurement a dozen times,
// which is the master's database growing for nothing : the very cost the choice
// of keeping these series here was weighed against.
//
// Kept in memory rather than asked of the database : it is one comparison per
// push against a query per push, and the worst a restart can cost is one
// measurement written twice, which the reading averages away.
var lastPushedMetric = struct {
	sync.Mutex
	at map[string]int64
}{at: make(map[string]int64)}

// alreadyRecorded reports whether this measurement has been seen, and remembers
// it when it has not.
func alreadyRecorded(hostname string, probe string, at int64) bool {
	if at == 0 {
		return false
	}

	lastPushedMetric.Lock()
	defer lastPushedMetric.Unlock()

	key := hostname + "\x00" + probe

	if last, known := lastPushedMetric.at[key]; known && at <= last {
		return true
	}

	lastPushedMetric.at[key] = at

	return false
}

// ForgetPushedMetrics drops what is remembered about a host, so a client that
// goes away does not hold a key forever.
func ForgetPushedMetrics(hostname string) {
	lastPushedMetric.Lock()
	defer lastPushedMetric.Unlock()

	for key := range lastPushedMetric.at {
		if strings.HasPrefix(key, hostname+"\x00") {
			delete(lastPushedMetric.at, key)
		}
	}
}

// RecordPushedMetrics keeps what a pushing client just measured.
//
// A client cannot be asked anything -- it sits behind a NAT -- so the only
// moment its measurements can be written down is the moment they arrive. What
// it sends is the same probe results a polled remote would answer with, so
// nothing new travels : this is a place to put them, not a new thing to send.
func RecordPushedMetrics(remote *Wigo) {
	if remote == nil || remote.LocalHost == nil {
		return
	}

	hostname := remote.GetHostname()
	if hostname == "" || hostname == GetLocalWigo().GetHostname() {
		return
	}

	for item := range remote.LocalHost.Probes.IterBuffered() {
		probe, ok := item.Val.(*ProbeResult)
		if !ok {
			continue
		}

		// The same result arrives on every push until the probe runs again
		if alreadyRecorded(hostname, probe.Name, probe.Timestamp) {
			continue
		}

		RecordProbeMetricsOf(hostname, probe)
	}
}
