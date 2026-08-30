package wigo

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// When things changed, and to what.
//
// The logs table already carries a sentence -- "Probe check_load switched from
// 100 to 300 : load too high" -- and a timeline could be built by parsing it.
// It would also break the day somebody rewords that sentence, silently, which
// is the worst way for a monitoring feature to fail. So the transition is
// written down as what it is : two statuses and a time.
//
// Unlike the metrics, this is recorded for everything a wigo observes, its
// remotes included. A status change is not a sample : there are a handful a
// day, not one a minute, so there is nothing to save by not storing them. And
// a master sees things a host cannot record about itself -- going down being
// the obvious one.

const defaultStatusHistoryDays = 30

// What a status was before it existed, or after it stopped. Not a status on the
// scale, and deliberately not zero : zero used to be what an empty host
// computed, and reusing it would make "never seen" and "no probe" the same.
const StatusAbsent = -1

const createStatusChangesTable = `
    CREATE TABLE IF NOT EXISTS status_changes (
        id integer not null primary key,
        host text not null,
        probe text not null,
        grp text,
        was int not null,
        now int not null,
        message text,
        at int not null
    ) ;
    CREATE INDEX IF NOT EXISTS status_changes_lookup ON status_changes(host, probe, at) ;
    `

// StatusChange is one transition.
type StatusChange struct {
	Host string

	// The probe, or empty when it is the host itself going up or down.
	Probe string

	Group string

	// StatusAbsent on Was means it appeared, on Now means it went away.
	Was int
	Now int

	Message string
	At      int64
}

func statusHistoryDays() int {
	days := GetLocalWigo().GetConfig().Global.StatusHistoryDays
	if days < 0 {
		return 0
	}

	return days
}

// RecordStatusTransition writes down that something changed.
//
// Failing is logged and goes no further : the change has already been noticed,
// notified about and displayed, and losing a line of history is not a reason to
// fail any of that.
func RecordStatusTransition(change StatusChange) {
	if statusHistoryDays() == 0 || change.Host == "" {
		return
	}
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return
	}

	if change.At == 0 {
		change.At = time.Now().Unix()
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	if _, err := LocalWigo.sqlLiteConn.Exec(
		`INSERT INTO status_changes(host,probe,grp,was,now,message,at) VALUES(?,?,?,?,?,?,?);`,
		change.Host, change.Probe, change.Group, change.Was, change.Now,
		change.Message, change.At); err != nil {
		log.Printf("Unable to record the status change of %s : %s", change.Host, err)
	}
}

// StatusTimeline is what the timeline endpoint answers.
type StatusTimeline struct {
	Host  string
	Probe string

	Since int64
	Until int64

	// What it was when the window opened, which is not in Changes : a host that
	// has been critical for a week has no transition inside the last day, and a
	// timeline that started blank would draw that week as nothing at all.
	StatusAtStart int

	// How many days this host keeps. Zero means it keeps none, which is why an
	// empty timeline is not the same as a thing that never changed.
	HistoryDays int

	Changes []StatusChange
}

// ProbeTimeline reads what happened to one probe, or to a whole host when probe
// is empty.
func ProbeTimeline(host string, probe string, since int64, until int64) (StatusTimeline, error) {
	timeline := StatusTimeline{
		Host:          host,
		Probe:         probe,
		Since:         since,
		Until:         until,
		StatusAtStart: StatusAbsent,
		HistoryDays:   statusHistoryDays(),
		Changes:       make([]StatusChange, 0),
	}

	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return timeline, fmt.Errorf("no database to read the timeline from")
	}
	if host == "" {
		return timeline, fmt.Errorf("no host to read the timeline of")
	}
	if probe != "" && !IsValidProbeName(probe) {
		return timeline, fmt.Errorf("invalid probe name %q", probe)
	}
	if since <= 0 || since >= until {
		return timeline, fmt.Errorf("the window has to start before it ends")
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	// What it was when the window opened : the last thing it became before it
	if err := LocalWigo.sqlLiteConn.QueryRow(
		`SELECT now FROM status_changes
		 WHERE host = ? AND probe = ? AND at < ?
		 ORDER BY at DESC LIMIT 1;`, host, probe, since).Scan(&timeline.StatusAtStart); err != nil {
		timeline.StatusAtStart = StatusAbsent
	}

	rows, err := LocalWigo.sqlLiteConn.Query(
		`SELECT host,probe,grp,was,now,message,at FROM status_changes
		 WHERE host = ? AND probe = ? AND at >= ? AND at <= ?
		 ORDER BY at;`, host, probe, since, until)
	if err != nil {
		return timeline, err
	}
	defer rows.Close()

	for rows.Next() {
		var change StatusChange
		if err := rows.Scan(&change.Host, &change.Probe, &change.Group,
			&change.Was, &change.Now, &change.Message, &change.At); err != nil {
			continue
		}
		timeline.Changes = append(timeline.Changes, change)
	}

	return timeline, rows.Err()
}

// StartStatusHistoryRetention drops what has aged out.
func StartStatusHistoryRetention() {
	days := statusHistoryDays()
	if days == 0 {
		log.Printf("Timeline : status changes are not kept, set StatusHistoryDays to keep them")
		return
	}

	log.Printf("Timeline : keeping %d days of status changes", days)

	go func() {
		for {
			dropAgedStatusChanges()
			time.Sleep(time.Hour)
		}
	}()
}

func dropAgedStatusChanges() {
	days := statusHistoryDays()
	if days == 0 || LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return
	}

	oldest := time.Now().Unix() - int64(days)*86400

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	if _, err := LocalWigo.sqlLiteConn.Exec(
		`DELETE FROM status_changes WHERE at < ?;`, oldest); err != nil {
		log.Printf("Fail to drop the aged status changes : %s", err)
	}
}

// HttpHostTimelineHandler answers the timeline of a host, or of one of its
// probes with ?probe=name.
//
// Answered from what this wigo observed, never forwarded : a master watching a
// remote saw things the remote cannot have recorded about itself, going down
// being the obvious one.
func HttpHostTimelineHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	hostname := r.PathValue("hostname")
	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	since, until, err := parseTimelineWindow(r)
	if err != nil {
		return 400, err.Error()
	}

	timeline, err := ProbeTimeline(hostname, r.URL.Query().Get("probe"), since, until)
	if err != nil {
		return 400, err.Error()
	}

	body, err := json.Marshal(timeline)
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the timeline : %s", err)
	}

	return 200, string(body)
}

func parseTimelineWindow(r *http.Request) (int64, int64, error) {
	now := time.Now().Unix()

	since := now - 86400
	if raw := r.URL.Query().Get("since"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid since %q, expected a unix timestamp", raw)
		}
		since = value
	}

	until := now
	if raw := r.URL.Query().Get("until"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid until %q, expected a unix timestamp", raw)
		}
		until = value
	}

	return since, until, nil
}

// The overall status each host was last seen at, so a change can be told from a
// repetition. In memory rather than queried : this runs on every update of every
// host, and the worst a restart costs is one band starting again.
var lastHostStatus = struct {
	sync.Mutex
	at map[string]int
}{at: make(map[string]int)}

// RecordHostStatuses writes down every host of a tree whose overall status
// changed since it was last seen.
//
// A whole tree rather than the host being updated : a master of masters sees
// its own remotes' remotes only inside their trees, and those never pass
// through AddOrUpdateRemoteWigo. Hooking that alone left the hosts two levels
// down with an empty band, which is the shape of the bug this fixes.
func RecordHostStatuses(root *Wigo) {
	if root == nil {
		return
	}

	if root.LocalHost != nil {
		noteHostStatus(root.GetHostname(), root.LocalHost.Group, root.LocalHost.Status,
			root.GlobalMessage)
	}

	for item := range root.RemoteWigos.IterBuffered() {
		remote, ok := item.Val.(*Wigo)
		if !ok {
			continue
		}

		RecordHostStatuses(remote)
	}
}

// noteHostStatus records one host, and remembers where it now stands.
func noteHostStatus(hostname string, group string, status int, message string) {
	if hostname == "" {
		return
	}

	lastHostStatus.Lock()
	was, known := lastHostStatus.at[hostname]
	if !known {
		// Asked of the database the first time a host is seen in this process,
		// rather than assumed absent : a restart would otherwise write a fresh
		// "never watched" transition every time, cutting a band that never
		// stopped and putting a gap on the screen where nothing happened.
		was, known = lastRecordedHostStatus(hostname)
	}
	lastHostStatus.at[hostname] = status
	lastHostStatus.Unlock()

	if !known {
		// Where its band starts. Without it a host that has been fine since it
		// was added has no row at all, and its whole timeline reads as never
		// watched rather than as always fine.
		RecordStatusTransition(StatusChange{
			Host: hostname, Group: group,
			Was: StatusAbsent, Now: status, Message: message,
		})

		return
	}

	RecordHostStatusChange(hostname, group, was, status, message)
}

// lastRecordedHostStatus is what this host was last written down as, if it ever
// was. Called once per host per process, from under lastHostStatus.
func lastRecordedHostStatus(hostname string) (int, bool) {
	if LocalWigo == nil || LocalWigo.sqlLiteConn == nil {
		return 0, false
	}

	LocalWigo.sqlLiteLock.Lock()
	defer LocalWigo.sqlLiteLock.Unlock()

	var status int
	err := LocalWigo.sqlLiteConn.QueryRow(
		`SELECT now FROM status_changes
		 WHERE host = ? AND probe = ''
		 ORDER BY at DESC LIMIT 1;`, hostname).Scan(&status)

	return status, err == nil
}

// RecordHostStatusChange writes down a change in what a host is worth overall.
//
// These bands used to hold only Down and Up, so a host that never stopped
// answering had nothing at all on its timeline and the screen said "nothing
// known over that window" about a machine it had been watching for a month.
// What somebody reads there is the host's health -- the summary beside it says
// "not fine for two hours of it" -- and health is the worst of its probes, not
// merely whether it replied.
//
// Down and Up keep owning reachability : a change out of the sentinel they use
// is theirs to write, and recording it here as well would put two transitions
// at the same instant.
func RecordHostStatusChange(hostname string, group string, was int, now int, message string) {
	if hostname == "" || was == now {
		return
	}

	// The status a wigo carries while it is unreachable, cf. Wigo.Down
	const down = 999

	if was == down || now == down {
		return
	}

	RecordStatusTransition(StatusChange{
		Host: hostname, Group: group,
		Was: was, Now: now, Message: message,
	})
}
