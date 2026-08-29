package wigo

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func HttpRemotesHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	hostname := r.PathValue("hostname")

	if hostname != "" {
		remoteWigo := GetLocalWigo().FindRemoteWigoByHostname(hostname)
		if remoteWigo != nil {
			json, err := remoteWigo.ToJsonString()
			if err != nil {
				return 500, "Failed to encode remote wigo"
			} else {
				return 200, json
			}
		} else {
			return 404, ""
		}
	}

	// Narrowed by label when asked. Without the parameter this is the list it
	// has always been : a filter nobody filled in must not change the answer.
	if asked := r.URL.Query().Get("labels"); asked != "" {
		selector, err := ParseSelector(asked)
		if err != nil {
			return 400, err.Error()
		}

		json, err := json.Marshal(GetLocalWigo().HostsMatching(selector))
		if err != nil {
			return 500, ""
		}

		return 200, string(json)
	}

	// Return remotes list
	list := GetLocalWigo().ListRemoteWigosNames()
	json, err := json.Marshal(list)
	if err != nil {
		return 500, ""
	} else {
		return 200, string(json)
	}
}

// HttpLabelsHandler answers every label in use with how many hosts carry each
// value, which is what a filter on screen is built from.
func HttpLabelsHandler(w http.ResponseWriter, r *http.Request) (int, string) {
	encoded, err := json.Marshal(GetLocalWigo().FleetLabels())
	if err != nil {
		return 500, "Failed to encode the labels"
	}

	return 200, string(encoded)
}

func HttpRemotesProbesHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	hostname := r.PathValue("hostname")
	probeName := r.PathValue("probe")

	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	// Get remote wigo
	remoteWigo := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remoteWigo == nil {
		return 404, "Remote wigo " + hostname + " not found"
	}

	// Get probe or probes
	if probeName != "" {
		if tmp, ok := remoteWigo.LocalHost.Probes.Get(probeName); ok {
			probe := tmp.(*ProbeResult)

			json, err := json.Marshal(probe)
			if err != nil {
				return 500, ""
			} else {
				return 200, string(json)
			}
		}
	} else {
		json, err := json.Marshal(remoteWigo.ListProbes())
		if err != nil {
			return 500, ""
		} else {
			return 200, string(json)
		}
	}

	return 200, ""
}

func HttpRemotesStatusHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	hostname := r.PathValue("hostname")

	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	// Get remote wigo
	remoteWigo := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remoteWigo == nil {
		return 404, "Remote wigo " + hostname + " not found"
	}

	return 200, strconv.Itoa(remoteWigo.GlobalStatus)
}

func HttpRemotesProbesStatusHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	hostname := r.PathValue("hostname")
	probeName := r.PathValue("probe")

	if hostname == "" {
		return 404, "No wigo name set in url"
	}
	if probeName == "" {
		return 404, "No probe name set in url"
	}

	// Get remote wigo
	remoteWigo := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remoteWigo == nil {
		return 404, "Remote wigo " + hostname + " not found"
	}

	// Get probe
	if tmp, ok := remoteWigo.LocalHost.Probes.Get(probeName); ok {
		probe := tmp.(*ProbeResult)
		return 200, strconv.Itoa(probe.Status)
	} else {
		return 404, "Probe " + probeName + " not found in remote wigo " + hostname
	}

}

func HttpLogsHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	//Parse url
	u, err := url.Parse(r.URL.String())
	if err != nil {
		return 500, fmt.Sprintf("%s", err)
	}
	pq, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return 500, fmt.Sprintf("%s", err)
	}

	// Get params
	hostname := ""
	if len(pq["hostname"]) > 0 {
		hostname = pq["hostname"][0]
	}
	probeName := ""
	if len(pq["probe"]) > 0 {
		probeName = pq["probe"][0]
	}
	group := ""
	if len(pq["group"]) > 0 {
		group = pq["group"][0]
	}

	// Index && Offset
	limit := 100
	if len(pq["limit"]) > 0 {
		if iInt, err := strconv.Atoi(pq["limit"][0]); err == nil {
			limit = iInt
		}
	}
	offset := 0
	if len(pq["offset"]) > 0 {
		if oInt, err := strconv.Atoi(pq["offset"][0]); err == nil {
			offset = oInt
		}
	}

	// Test hostname if present
	var remoteWigo *Wigo
	if hostname != "" {
		remoteWigo = GetLocalWigo().FindRemoteWigoByHostname(hostname)
		if remoteWigo == nil {
			return 404, "Remote wigo " + hostname + " not found"
		}
	}

	// Test probe
	if probeName != "" {
		if hostname != "" {
			// Get probe
			if _, ok := remoteWigo.LocalHost.Probes.Get(probeName); ok {
			} else {
				return 404, "Probe " + probeName + " not found in remote wigo " + hostname
			}
		}
	}

	// Get logs
	logs := LocalWigo.SearchLogs(probeName, hostname, group, uint64(limit), uint64(offset))

	// Json
	json, err := json.Marshal(logs)
	if err != nil {
		return 500, ""
	}

	return 200, string(json)
}

func HttpGroupsHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	group := r.PathValue("group")

	result := make(map[string]interface{})
	result["Name"] = group

	if group != "" {
		gs, status := GetLocalWigo().GroupSummary(group)
		if gs != nil {

			result["Status"] = status
			result["Hosts"] = gs

			json, err := json.Marshal(result)
			if err != nil {
				return 500, fmt.Sprintf("Fail to encode summary : %s", err)
			} else {
				return 200, string(json)
			}
		} else {
			return 404, ""
		}
	}

	// Return remotes list
	list := GetLocalWigo().ListGroupsNames()
	json, err := json.Marshal(list)
	if err != nil {
		return 500, ""
	} else {
		return 200, string(json)
	}
}

func HttpLogsIndexesHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	result := make(map[string][]string)
	result["probes"] = make([]string, 0)
	result["hosts"] = make([]string, 0)
	result["groups"] = make([]string, 0)

	// Queries
	qP := "SELECT DISTINCT(probe) FROM logs"
	qH := "SELECT DISTINCT(host) FROM logs"
	qG := "SELECT DISTINCT(grp) FROM logs"

	// Probes
	if rowsProbes, err := LocalWigo.sqlLiteConn.Query(qP); err == nil {
		for rowsProbes.Next() {
			var p string
			if err := rowsProbes.Scan(&p); err == nil {
				result["probes"] = append(result["probes"], p)
			}
		}
	}

	// Hosts
	if rowsHosts, err := LocalWigo.sqlLiteConn.Query(qH); err == nil {
		for rowsHosts.Next() {
			var h string
			if err := rowsHosts.Scan(&h); err == nil {
				result["hosts"] = append(result["hosts"], h)
			}
		}
	}

	// Groups
	if rowsGroup, err := LocalWigo.sqlLiteConn.Query(qG); err == nil {
		for rowsGroup.Next() {
			var g string
			if err := rowsGroup.Scan(&g); err == nil {
				result["groups"] = append(result["groups"], g)
			}
		}
	}

	// Return remotes list
	json, err := json.Marshal(result)
	if err != nil {
		return 500, fmt.Sprintf("Error while encoding to json : %s", err)
	} else {
		return 200, string(json)
	}
}

// The shape of this answer is deliberately left alone. Adding a field to say
// whether the caller may act on it would have been consistent with the rest of
// the api, and would also have broken every client decoding it into a
// map[string]map[string]string, which is what its two keys invite. The
// interface asks /api/whoami instead -- and the role is the more accurate
// question here anyway, since admitting a client does not depend on
// AllowWriteActions.
func HttpAuthorityListHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	result := make(map[string]map[string]string)

	if LocalWigo.push == nil {
		return 500, "Push server is not started"
	}

	result["waiting"] = LocalWigo.push.authority.Waiting
	result["allowed"] = LocalWigo.push.authority.Allowed

	// Return remotes list
	json, err := json.Marshal(result)
	if err != nil {
		return 500, fmt.Sprintf("Error while encoding to json : %s", err)
	} else {
		return 200, string(json)
	}
}

// httpAuthorityAllowed reports whether this request may admit or expel a client.
//
// The role only, deliberately not httpWriteActionsAllowed. AllowWriteActions is
// about letting the api change *this host* -- enable, disable and repitch its
// own probes -- and a push master accepting a client is not that. Requiring it
// would stop every existing push master, since it is off by default, from doing
// what it has always done.
//
// The role check on its own changes nothing for any configuration that existed
// before : a wigo with no Login served everybody as an operator, and one with a
// Login refused everybody without it. It closes exactly one hole, the one
// opened by letting anonymous callers read.
func httpAuthorityAllowed(r *http.Request) (int, string, bool) {
	if caller := CallerOf(r); !caller.May(RoleOperator) {
		if caller.Name == "anonymous" {
			return 403, "This host is open for reading only. Sign in, or present an operator token, to admit or revoke a client.", false
		}

		return 403, "This credential is read only. An operator token is needed to admit or revoke a client.", false
	}

	return 0, "", true
}

func HttpAuthorityAllowHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	// Admitting a client lets an unknown machine push into this master, which
	// is the most consequential write this api has.
	if status, message, allowed := httpAuthorityAllowed(r); !allowed {
		return status, message
	}

	uuid := r.PathValue("uuid")

	if LocalWigo.push == nil {
		return 500, "Push server is not started"
	}

	err := LocalWigo.push.authority.AllowClient(uuid)

	if err != nil {
		return 500, err.Error()
	}

	return 200, "OK"
}

func HttpAuthorityRevokeHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	// And expelling one stops a machine being monitored at all, silently
	if status, message, allowed := httpAuthorityAllowed(r); !allowed {
		return status, message
	}

	uuid := r.PathValue("uuid")

	if LocalWigo.push == nil {
		return 500, "Push server is not started"
	}

	err := LocalWigo.push.authority.RevokeClient(uuid)

	if err != nil {
		return 500, err.Error()
	}

	return 200, "OK"
}

// Probes scheduling
//
// The probes directory is the source of truth, so these handlers read it and
// write to it directly rather than going through any cached state.

// ProbesSchedule is what GET /api/probes answers. The hostname and the write
// flag come along so a client knows which host these probes belong to, and
// whether acting on them would be refused, without having to try.
type ProbesSchedule struct {
	Hostname            string
	WriteActionsAllowed bool

	// Why not, when it is false. Said here because only this side can tell the
	// three cases apart : the caller's role, this host refusing writes, and a
	// pushing client that has not opted into being driven -- and each is fixed
	// in a different file. A screen that names the wrong one sends somebody
	// editing a setting that was never the problem.
	//
	// Empty when writes are allowed, and empty from an older wigo, which is why
	// the interface keeps a fallback sentence.
	ReadOnlyReason string `json:",omitempty"`

	Probes []ProbeLocation

	// Probes that ran, exited with the special code 13 and asked not to be run
	// again -- usually because there is nothing for them to check on this host,
	// check_mdadm on a machine with no raid array being the typical case. They
	// are scheduled and produce no result, which is indistinguishable from
	// never having run unless it is said. Cleared by a restart.
	SkippedProbes []string

	// Why somebody turned a probe off, and until when. Only the probes an
	// operator actually decided about are in here : most disabled probes were
	// simply never enabled, and attributing those to somebody would be a lie.
	DisableRecords []ProbeDisableRecord
}

// HttpProbesHandler lists the probes of this host with their schedule,
// including the ones that are currently disabled and therefore have no result.
func HttpProbesHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	locations, err := ProbeLocations()
	if err != nil {
		return 500, fmt.Sprintf("Fail to read the probes directory : %s", err)
	}

	// The caller's role counts as much as the host's flag : offering a control
	// that always answers 403 is worse than not offering it.
	_, refusal, mayWrite := httpWriteActionsAllowed(r)

	schedule := ProbesSchedule{
		Hostname:            GetLocalWigo().GetHostname(),
		WriteActionsAllowed: mayWrite,
		ReadOnlyReason:      refusal,
		Probes:              locations,
		SkippedProbes:       GetLocalWigo().GetDisabledProbes(),
		DisableRecords:      ProbeDisableRecords(),
	}

	body, err := json.Marshal(schedule)
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the probes list : %s", err)
	}

	return 200, string(body)
}

// httpWriteActionsAllowed reports whether this request may change anything.
//
// Two independent questions, and both have to be yes. The host has to accept
// being changed at all, which is a decision about the machine. And the caller
// has to be an operator, which is a decision about them : handing somebody the
// dashboard so they can look at a graph must not also hand them the ability to
// switch the monitoring off.
func httpWriteActionsAllowed(r *http.Request) (int, string, bool) {

	if !GetLocalWigo().GetConfig().Http.AllowWriteActions {
		return 403, "Write actions are disabled on this host. Set AllowWriteActions in the [Http] section of the configuration file to allow them.", false
	}

	if caller := CallerOf(r); !caller.May(RoleOperator) {
		// Told apart on purpose : somebody holding a read-only token has to
		// swap it, somebody who presented nothing has to present something,
		// and the same sentence for both sends half of them looking in the
		// wrong place.
		if caller.Name == "anonymous" {
			return 403, "This host is open for reading only. Sign in, or present an operator token, to change anything.", false
		}

		return 403, "This credential is read only. An operator token is needed to change anything.", false
	}

	return 0, "", true
}

// HttpProbeDisableHandler stops a probe from being scheduled.
func HttpProbeDisableHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	if status, message, allowed := httpWriteActionsAllowed(r); !allowed {
		return status, message
	}

	probeName := r.PathValue("probe")
	if probeName == "" {
		return 404, "No probe name set in url"
	}

	duration, err := parseDisableDuration(r.URL.Query().Get("for"))
	if err != nil {
		return 400, err.Error()
	}

	if err := DisableProbeWithReason(probeName, r.URL.Query().Get("reason"),
		httpAuthor(r, r.URL.Query().Get("author")), duration); err != nil {
		return 400, err.Error()
	}

	return HttpProbesHandler(w, r)
}

// DisableProbeWithReason turns a probe off and notes why, for how long and at
// whose request. Shared by the API and by an order coming from a master.
func DisableProbeWithReason(probeName string, reason string, author string, duration int64) error {

	// Captured before the probe stops : once it is disabled there is nothing
	// left to read the interval from, and an expiring disable needs to know
	// what to put the probe back to.
	interval := scheduledIntervalOf(probeName)

	if duration > 0 && interval < MinProbeInterval {
		return fmt.Errorf("probe %q is not running, so there is no interval to bring it back to "+
			"when the disable expires. Disable it without a duration, or give it an interval first.", probeName)
	}

	if err := UnscheduleProbe(probeName); err != nil {
		return err
	}

	record := ProbeDisableRecord{
		Probe:     probeName,
		Reason:    reason,
		Author:    author,
		Interval:  interval,
		CreatedAt: time.Now().Unix(),
	}
	if duration > 0 {
		record.Until = record.CreatedAt + duration
	}

	// The probe is already stopped. Failing to take a note about it must not
	// turn a successful disable into an error, or the interface would end up
	// disagreeing with the disk.
	if err := recordProbeDisabled(record); err != nil {
		log.Printf("Probe %s has been disabled but no record could be kept of it : %s", probeName, err)
	}

	GetLocalWigo().AddLog(nil, INFO, describeDisable(record))

	return nil
}

// describeDisable is what ends up in the logs table, and it is the only trace
// left if the record itself cannot be written.
func describeDisable(record ProbeDisableRecord) string {
	message := fmt.Sprintf("Probe %s has been disabled through the API by %s",
		record.Probe, describeAuthor(record.Author))

	if record.Reason != "" {
		message += fmt.Sprintf(" : %s", record.Reason)
	}
	if record.Until > 0 {
		message += fmt.Sprintf(" (until %s)", time.Unix(record.Until, 0).Format(dateLayout))
	}

	return message
}

// scheduledIntervalOf returns the interval a probe currently runs at, or zero
// when nothing schedules it. The shortest one wins if it somehow runs at
// several : that is the pace it actually keeps.
func scheduledIntervalOf(probeName string) int {
	locations, err := FindProbeLocations(probeName)
	if err != nil {
		return 0
	}

	interval := 0
	for _, location := range locations {
		if !location.Enabled {
			continue
		}
		if interval == 0 || location.Interval < interval {
			interval = location.Interval
		}
	}

	return interval
}

// parseDisableDuration reads how long a disable is meant to last. Empty means
// until somebody turns it back on.
func parseDisableDuration(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q, expected something like 1h, 12h or 168h", value)
	}

	if duration < time.Minute || duration > maxDisableDuration {
		return 0, fmt.Errorf("a disable must last between a minute and a year, got %s", duration)
	}

	return int64(duration.Seconds()), nil
}

// A year. Past that it is not a temporary disable, and pretending otherwise
// would put a deadline nobody will ever see on a permanent decision.
const maxDisableDuration = 365 * 24 * time.Hour

// httpAuthor records who asked, as far as that can be known.
//
// There is no identity system : the API sits behind one shared basic auth
// credential, so the honest answer is the login that was used and the address
// it came from. F8 replaces this with a real author.
//
// A master driving this host forwards the author it recorded on its own side,
// because the operator clicked there and not here. That is a claim, not a fact
// -- the credential is shared and anyone able to reach this endpoint could send
// any string -- so it is kept alongside who actually connected rather than
// instead of it.
func httpAuthor(r *http.Request, claimed string) string {
	connected := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		connected = forwarded
	}

	if caller := CallerOf(r); caller.Name != "" && caller.Name != "unauthenticated" {
		connected = fmt.Sprintf("%s from %s", caller.Name, connected)
	}

	if claimed != "" {
		return fmt.Sprintf("%s via %s", claimed, connected)
	}

	return connected
}

// HttpProbeIntervalHandler sets how often a probe runs, scheduling it again
// when it was disabled.
func HttpProbeIntervalHandler(w http.ResponseWriter, r *http.Request) (int, string) {

	if status, message, allowed := httpWriteActionsAllowed(r); !allowed {
		return status, message
	}

	probeName := r.PathValue("probe")
	if probeName == "" {
		return 404, "No probe name set in url"
	}

	seconds := r.URL.Query().Get("seconds")
	if seconds == "" {
		return 400, "No interval set in url, expected ?seconds=300"
	}

	interval, err := parseProbeInterval(seconds)
	if err != nil {
		return 400, err.Error()
	}

	if err := ScheduleProbe(probeName, interval); err != nil {
		return 400, err.Error()
	}

	// It runs again, so whatever was noted about it being off is now false
	if err := forgetProbeDisabled(probeName); err != nil {
		log.Printf("Probe %s runs again but its disable record could not be dropped : %s", probeName, err)
	}

	GetLocalWigo().AddLog(nil, INFO, fmt.Sprintf("Probe %s is now scheduled every %d seconds through the API", probeName, interval))

	return HttpProbesHandler(w, r)
}
