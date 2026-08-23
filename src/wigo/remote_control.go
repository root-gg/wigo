package wigo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/codegangsta/martini"
)

// Acting on a remote host means asking that host to act on itself : the call is
// forwarded to its own API, and it applies its own Http.AllowWriteActions.
//
// Both ends have to agree. The remote decides whether it accepts the change,
// and there is no way for a master to bypass that : a remote cannot tell a
// forwarded call from an administrator running curl, so its own gate is the
// only one it could enforce. This host decides whether it forwards at all,
// because an administrator who turned write actions off here expects it to
// perform none, not to remain usable as a jump host onto the whole fleet.
//
// Hosts that push to us are the other way round : we cannot reach them, so they
// have to come and ask for their orders, and PushClient.AllowRemoteControl is
// how each of them says whether it ever will.

// remoteEndpoint is how to reach a remote this wigo polls.
//
// It holds credentials, so it is kept out of the Wigo struct on purpose : that
// struct is marshalled to JSON on every /api call and to gob on every push, and
// a password has no business travelling with it.
type remoteEndpoint struct {
	baseUrl  string
	login    string
	password string
}

var remoteEndpoints = struct {
	sync.RWMutex
	byUuid map[string]remoteEndpoint
}{byUuid: make(map[string]remoteEndpoint)}

// Answers are read with a ceiling : a remote is not necessarily healthy, and
// this runs while an operator waits.
const maxRemoteAnswerSize = 1 << 20

var remoteControlClient = http.Client{Timeout: 10 * time.Second}

// RegisterRemoteEndpoint records where a remote answered, so a later call can
// be forwarded to it. Called by the polling routine once it knows the uuid.
func RegisterRemoteEndpoint(uuid string, baseUrl string, login string, password string) {
	if uuid == "" || baseUrl == "" {
		return
	}

	remoteEndpoints.Lock()
	defer remoteEndpoints.Unlock()

	remoteEndpoints.byUuid[uuid] = remoteEndpoint{
		baseUrl:  baseUrl,
		login:    login,
		password: password,
	}
}

func remoteEndpointFor(uuid string) (remoteEndpoint, bool) {
	remoteEndpoints.RLock()
	defer remoteEndpoints.RUnlock()

	endpoint, known := remoteEndpoints.byUuid[uuid]
	return endpoint, known
}

// forwardWriteToRemote passes a write on to the host named hostname and hands
// its answer back untouched.
//
// Both hosts have to agree : this one to act as a control plane at all, and the
// remote to accept the change. An administrator who turned write actions off
// here expects this wigo not to perform any, and being usable as a jump host to
// reconfigure the whole fleet would make a lie of that.
func forwardWriteToRemote(hostname string, method string, path string, query url.Values) (int, string) {

	if !GetLocalWigo().GetConfig().Http.AllowWriteActions {
		return 403, "Write actions are disabled on this host, so it will not forward one either. " +
			"Set AllowWriteActions in the [Http] section of its configuration file to allow them."
	}

	return forwardToRemote(hostname, method, path, query)
}

// queueWriteForPushClient records an order for a host that pushes to us.
//
// We cannot call such a host, it sits behind a NAT, so the change is not
// applied now : it is handed over the next time the client asks for its orders.
// The answer says so rather than pretending the probe already moved.
func queueWriteForPushClient(hostname string, command ProbeCommand) (int, string, bool) {

	remote := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remote == nil || GetLocalWigo().push == nil {
		return 0, "", false
	}

	// A host we poll is driven over its own API instead, synchronously
	if _, polled := remoteEndpointFor(remote.Uuid); polled {
		return 0, "", false
	}

	if !GetLocalWigo().push.authority.IsAllowed(remote.Uuid) {
		return 0, "", false
	}

	if !GetLocalWigo().GetConfig().Http.AllowWriteActions {
		return 403, "Write actions are disabled on this host, so it will not drive a client either. " +
			"Set AllowWriteActions in the [Http] section of its configuration file to allow them.", true
	}

	if !ClientAcceptsRemoteControl(remote.Uuid) {
		return 403, fmt.Sprintf("%s does not accept being driven from here. "+
			"Set AllowRemoteControl in the [PushClient] section of its own configuration file, "+
			"it tells us on its next push.", hostname), true
	}

	if err := QueueProbeCommand(remote.Uuid, command); err != nil {
		return 400, err.Error(), true
	}

	return 202, fmt.Sprintf("Queued for %s. It pushes to this host, so it cannot be called : "+
		"the change is applied the next time it asks for its orders.", hostname), true
}

// forwardToRemote passes a call on to the host named hostname and hands its
// answer back untouched.
func forwardToRemote(hostname string, method string, path string, query url.Values) (int, string) {

	remote := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remote == nil {
		return 404, "Remote wigo " + hostname + " not found"
	}

	endpoint, known := remoteEndpointFor(remote.Uuid)
	if !known {
		return 501, fmt.Sprintf("%s cannot be reached from here : this wigo neither polls it nor receives "+
			"its pushes. A host sitting behind another wigo is only reachable from the one that polls it.", hostname)
	}

	target := endpoint.baseUrl + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		return 500, fmt.Sprintf("Fail to build the request for %s : %s", hostname, err)
	}

	if endpoint.login != "" && endpoint.password != "" {
		req.SetBasicAuth(endpoint.login, endpoint.password)
	}

	resp, err := remoteControlClient.Do(req)
	if err != nil {
		return 502, fmt.Sprintf("Fail to reach %s : %s", hostname, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteAnswerSize))
	if err != nil {
		return 502, fmt.Sprintf("Fail to read the answer from %s : %s", hostname, err)
	}

	// The remote's own answer goes straight through. When it refuses because
	// its AllowWriteActions is off, the operator has to read that sentence and
	// not a generic failure invented here.
	return resp.StatusCode, string(body)
}

// probeApiPath builds the path of a probe endpoint on another wigo. The name is
// validated before it is put in a url, never after.
func probeApiPath(probeName string, action string) (string, error) {
	if !IsValidProbeName(probeName) {
		return "", fmt.Errorf("invalid probe name %q", probeName)
	}

	return "/api/probes/" + url.PathEscape(probeName) + "/" + action, nil
}

// HttpHostScheduleHandler lists the probes of any host of the tree with their
// schedule. Answers for this host directly and forwards for the others.
func HttpHostScheduleHandler(params martini.Params) (int, string) {

	hostname := params["hostname"]
	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	if hostname == GetLocalWigo().GetHostname() {
		return HttpProbesHandler()
	}

	if status, body, isPushClient := pushClientSchedule(hostname); isPushClient {
		return status, body
	}

	return forwardToRemote(hostname, "GET", "/api/probes", nil)
}

// pushClientSchedule answers for a host that pushes to us, from what it last
// reported. We cannot call such a host, so this is the only thing we have.
func pushClientSchedule(hostname string) (int, string, bool) {

	remote := GetLocalWigo().FindRemoteWigoByHostname(hostname)
	if remote == nil || GetLocalWigo().push == nil {
		return 0, "", false
	}

	// A host we poll is asked directly instead, which is always fresher
	if _, polled := remoteEndpointFor(remote.Uuid); polled {
		return 0, "", false
	}

	if !GetLocalWigo().push.authority.IsAllowed(remote.Uuid) {
		return 0, "", false
	}

	locations, skipped, reported := ClientProbesSchedule(remote.Uuid)
	if !reported {
		return 501, fmt.Sprintf("%s pushes to this host but has not reported its probes schedule. "+
			"It runs a version that does not send one yet, so a probe it has disabled cannot be seen from here.",
			hostname), true
	}

	// What decides whether this host may be changed from here is its own
	// AllowRemoteControl, not the AllowWriteActions of its local API.
	schedule := ProbesSchedule{
		Hostname:            hostname,
		WriteActionsAllowed: ClientAcceptsRemoteControl(remote.Uuid),
		Probes:              locations,
		SkippedProbes:       skipped,
	}

	body, err := json.Marshal(schedule)
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the probes list : %s", err), true
	}

	return 200, string(body), true
}

// HttpHostProbeDisableHandler stops a probe from being scheduled on any host of
// the tree.
func HttpHostProbeDisableHandler(params martini.Params) (int, string) {

	hostname := params["hostname"]
	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	if hostname == GetLocalWigo().GetHostname() {
		return HttpProbeDisableHandler(params)
	}

	if !IsValidProbeName(params["probe"]) {
		return 400, fmt.Sprintf("invalid probe name %q", params["probe"])
	}

	if status, message, isPushClient := queueWriteForPushClient(hostname, ProbeCommand{
		Action: CommandDisableProbe,
		Probe:  params["probe"],
	}); isPushClient {
		return status, message
	}

	path, err := probeApiPath(params["probe"], "disable")
	if err != nil {
		return 400, err.Error()
	}

	return forwardWriteToRemote(hostname, "POST", path, nil)
}

// HttpHostProbeIntervalHandler sets how often a probe runs on any host of the
// tree.
func HttpHostProbeIntervalHandler(params martini.Params, r *http.Request) (int, string) {

	hostname := params["hostname"]
	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	if hostname == GetLocalWigo().GetHostname() {
		return HttpProbeIntervalHandler(params, r)
	}

	if !IsValidProbeName(params["probe"]) {
		return 400, fmt.Sprintf("invalid probe name %q", params["probe"])
	}

	// Checked here too so an obviously wrong value never leaves this host, even
	// though the host that applies it decides for itself.
	seconds := r.URL.Query().Get("seconds")
	if seconds == "" {
		return 400, "No interval set in url, expected ?seconds=300"
	}
	interval, err := parseProbeInterval(seconds)
	if err != nil {
		return 400, err.Error()
	}

	if status, message, isPushClient := queueWriteForPushClient(hostname, ProbeCommand{
		Action:   CommandSetProbeInterval,
		Probe:    params["probe"],
		Interval: interval,
	}); isPushClient {
		return status, message
	}

	path, err := probeApiPath(params["probe"], "interval")
	if err != nil {
		return 400, err.Error()
	}

	return forwardWriteToRemote(hostname, "POST", path, url.Values{"seconds": []string{seconds}})
}
