package wigo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/codegangsta/martini"
)

// Running a probe out of band, because somebody asked rather than because its
// interval came round.
//
// The point is the wait after a fix : an hourly probe that just went critical
// is an hour of not knowing whether the repair worked, and restarting wigo to
// find out is not an answer. It is also the only way back for a probe that
// exited 13 and took itself out of the rotation, which until now really did
// need a restart.
//
// Nothing about the schedule changes. The probe runs once, its result replaces
// the one on screen, and the next scheduled run happens as it would have.

// A probe normally gets its whole interval minus a second to answer. On demand
// that would mean holding an http request open for an hour, so the wait is
// capped -- generously, since a probe worth rechecking by hand is often one
// that talks to something slow.
const maxOnDemandProbeTimeout = 30

// probeRunner executes a probe and publishes its result. It is the binary that
// knows how to do that, not this package, so it hands the function over at
// startup.
var probeRunner = struct {
	sync.RWMutex
	run func(probePath string, interval int, timeout int)
}{}

// SetProbeRunner is called once by the binary at startup.
func SetProbeRunner(run func(probePath string, interval int, timeout int)) {
	probeRunner.Lock()
	defer probeRunner.Unlock()

	probeRunner.run = run
}

// RunProbeNow executes a probe immediately and waits for its result.
//
// Only a probe that is scheduled may be run : running a disabled one would put
// a result on screen for a check that is not happening, which is the one thing
// the whole disabled state exists to make visible.
func RunProbeNow(name string) error {
	if !IsValidProbeName(name) {
		return fmt.Errorf("invalid probe name %q", name)
	}

	probeRunner.RLock()
	run := probeRunner.run
	probeRunner.RUnlock()

	if run == nil {
		return fmt.Errorf("this wigo cannot run a probe on demand")
	}

	locations, err := FindProbeLocations(name)
	if err != nil {
		return err
	}

	// The pace it actually keeps, if it somehow runs at several
	interval := 0
	directory := ""
	for _, location := range locations {
		if !location.Enabled {
			continue
		}
		if interval == 0 || location.Interval < interval {
			interval = location.Interval
			directory = location.Directory
		}
	}

	if interval == 0 {
		return fmt.Errorf("probe %q is disabled, nothing schedules it. Give it an interval to run it", name)
	}

	path, err := probePath(probesRoot(), directory, name)
	if err != nil {
		return err
	}

	// A probe that bowed out with exit code 13 is skipped by the scheduler. Ask
	// for it explicitly and it gets its chance : if nothing changed it exits 13
	// again during this very run and takes itself back out.
	GetLocalWigo().ClearSkippedProbe(name)

	timeout := interval - 1
	if timeout > maxOnDemandProbeTimeout {
		timeout = maxOnDemandProbeTimeout
	}

	run(path, interval, timeout)

	return nil
}

// HttpProbeRunHandler runs a probe of this host now and answers its result.
func HttpProbeRunHandler(params martini.Params) (int, string) {

	if status, message, allowed := httpWriteActionsAllowed(); !allowed {
		return status, message
	}

	probeName := params["probe"]
	if probeName == "" {
		return 404, "No probe name set in url"
	}

	if err := RunProbeNow(probeName); err != nil {
		return 400, err.Error()
	}

	// Read back rather than returned by the run : a probe that exits 13 has its
	// result discarded on purpose, and saying so is more useful than inventing
	// one.
	result, found := GetLocalWigo().GetLocalHost().Probes.Get(probeName)
	if !found {
		return 200, fmt.Sprintf("Probe %s ran and produced no result. It exited with the special code 13, "+
			"which means it has nothing to check on this host.", probeName)
	}

	body, err := json.Marshal(result)
	if err != nil {
		return 500, fmt.Sprintf("Fail to encode the result of probe %s : %s", probeName, err)
	}

	GetLocalWigo().AddLog(nil, INFO, fmt.Sprintf("Probe %s has been rechecked through the API", probeName))

	return 200, string(body)
}

// HttpHostProbeRunHandler runs a probe of any host of the tree now.
func HttpHostProbeRunHandler(params martini.Params, r *http.Request) (int, string) {

	hostname := params["hostname"]
	if hostname == "" {
		return 404, "No wigo name set in url"
	}

	if hostname == GetLocalWigo().GetHostname() {
		return HttpProbeRunHandler(params)
	}

	if !IsValidProbeName(params["probe"]) {
		return 400, fmt.Sprintf("invalid probe name %q", params["probe"])
	}

	if status, message, isPushClient := queueWriteForPushClient(hostname, ProbeCommand{
		Action: CommandRunProbe,
		Probe:  params["probe"],
	}); isPushClient {
		return status, message
	}

	path, err := probeApiPath(params["probe"], "run")
	if err != nil {
		return 400, err.Error()
	}

	return forwardWriteToRemoteWith(&remoteRecheckClient, hostname, "POST", path, nil)
}
