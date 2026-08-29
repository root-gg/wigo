package main

import (
	"container/list"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/root-gg/wigo/src/wigo"

	"github.com/howeyc/fsnotify"
)

func main() {

	// Init Wigo
	err := wigo.InitWigo()
	if err != nil {
		log.Printf("Error initialising Wigo : %s", err)
		os.Exit(1)
	}

	config := wigo.GetLocalWigo().GetConfig()

	if config.Global.Debug {
		// Debug heap
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	// Launch goroutines
	go threadWatch(wigo.Channels.ChanWatch)
	go threadLocalChecks()
	go threadCallbacks(wigo.Channels.ChanCallbacks)
	go threadRemoteChecks(config.RemoteWigos.AdvancedList)

	// The binary owns probe execution, so it hands the package a way to run one
	// on demand. Synchronous : the caller is an http request waiting for the
	// result it just asked for.
	wigo.SetProbeRunner(execProbe)

	// A probe linked in two interval directories runs from the smaller one only.
	// The other links do nothing, and a link doing nothing is worth one line at
	// boot rather than an interval somebody spends an afternoon disbelieving.
	wigo.LogDuplicateSchedules()

	// A dependency rule naming a host this wigo does not watch does nothing at
	// all, and doing nothing at all is what a working dependency looks like.
	wigo.LogDependencyProblems()

	// Brings back the probes whose disable was meant to be temporary. It is the
	// only thing that reads that table to act, and it can only ever start a
	// probe, never stop one.
	wigo.StartDisabledProbesExpiry()

	// Says again what is still wrong. A problem that broke at 3am and is still
	// broken at 9am produced exactly one message without this.
	wigo.StartRenotify()

	// Drops what has aged out of the metrics history
	wigo.StartMetricsRetention()
	wigo.StartStatusHistoryRetention()

	if config.Http.Enabled {
		go threadHttp(config.Http)
	}

	if config.PushClient.Enabled {
		go threadPush(config.PushClient)
	}

	// Signals
	signal.Notify(wigo.Channels.ChanSignals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Selection
	for {
		select {

		case sig := <-wigo.Channels.ChanSignals:
			switch sig {
			case syscall.SIGHUP:
				log.Printf("Caught SIGHUP. Reloading logger filehandle and configuration file...\n")
				wigo.GetLocalWigo().InitOrReloadLogger()
			case syscall.SIGTERM:
				os.Exit(0)
			case os.Interrupt:
				os.Exit(0)
			}
		}
	}
}

//
//// Threads
//

func threadWatch(ci chan wigo.Event) {

	// Vars
	probeDirectories := make([]string, 0)

	// First list
	probeDirectories, err := wigo.ListProbesDirectories()

	// Send
	for _, dir := range probeDirectories {
		ci <- wigo.Event{Type: wigo.ADDDIRECTORY, Value: wigo.GetLocalWigo().GetConfig().Global.ProbesDirectory + "/" + dir}
	}

	// Init inotify
	watcherNew, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
		return
	}

	// Create a watcher on checks directory
	err = watcherNew.Watch(wigo.GetLocalWigo().GetConfig().Global.ProbesDirectory)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Watch for changes forever
	for {
		select {

		case ev := <-watcherNew.Event:

			if ev.IsCreate() {
				fileInfo, err := os.Stat(ev.Name)
				if err != nil {
					log.Printf("Error stating %s : %s", ev.Name, err)
					return
				}

				if fileInfo.IsDir() {
					ci <- wigo.Event{Type: wigo.ADDDIRECTORY, Value: ev.Name}
				}

			} else if ev.IsDelete() {
				ci <- wigo.Event{Type: wigo.REMOVEDIRECTORY, Value: ev.Name}
			} else if ev.IsRename() {
				ci <- wigo.Event{Type: wigo.REMOVEDIRECTORY, Value: ev.Name}
			}
		}
	}
}

func threadLocalChecks() {

	// Directories currently scheduled, each with the channel that stops it.
	// Written here by the event loop and read by the goroutines it spawns, so
	// it needs a lock : the previous version scanned a shared container/list
	// from every directory goroutine without one.
	activeDirectories := make(map[string]chan struct{})
	var activeDirectoriesLock sync.Mutex

	// Listen events
	go func() {
		for {
			ev := <-wigo.Channels.ChanWatch

			switch ev.Type {
			case wigo.ADDDIRECTORY:

				var directory string = ev.Value.(string)

				// A probes directory is named after its check interval in
				// seconds. Anything else (examples, disabled, ...) is not a
				// schedule and is skipped on purpose.
				interval, isSchedule := wigo.ProbeDirectoryInterval(path.Base(directory))
				if !isSchedule {
					log.Printf("Skipping directory %s : its name is not a check interval\n", directory)
					continue
				}

				activeDirectoriesLock.Lock()
				if _, alreadyScheduled := activeDirectories[directory]; alreadyScheduled {
					activeDirectoriesLock.Unlock()
					continue
				}
				stop := make(chan struct{})
				activeDirectories[directory] = stop
				activeDirectoriesLock.Unlock()

				log.Println("Adding directory", directory)
				go scheduleProbesDirectory(directory, interval, stop)

			case wigo.REMOVEDIRECTORY:

				var directory string = ev.Value.(string)

				activeDirectoriesLock.Lock()
				if stop, scheduled := activeDirectories[directory]; scheduled {
					log.Println("Removing directory ", directory)
					close(stop)
					delete(activeDirectories, directory)
				}
				activeDirectoriesLock.Unlock()
			}
		}
	}()
}

// scheduleProbesDirectory runs every probe of a directory, every interval
// seconds, until stop is closed.
func scheduleProbesDirectory(directory string, interval int, stop chan struct{}) {

	// Local view of the directory, to detect the probes that appear and go away.
	// Only what this directory owns : a probe also linked in a smaller interval
	// runs from there, and running it here as well would have two schedulers
	// overwrite each other's result at two different rates.
	currentProbesList, err := wigo.ProbesOwnedBy(directory)
	if err != nil {
		log.Printf("Fail to read directory %s : %s", directory, err)
		currentProbesList = list.New()
	}

	for {
		if newProbesList, err := wigo.ProbesOwnedBy(directory); err == nil {
			for _, probeName := range reconcileProbesList(directory, currentProbesList, newProbesList) {
				// Leaving this directory does not mean the probe is gone : it
				// may have been repitched to another interval, and that
				// directory owns its result now. Dropping it here would delete
				// a result the other one just produced.
				if wigo.IsProbeScheduled(probeName) {
					log.Printf("Probe %s has been moved to another interval, keeping its result\n", probeName)
					continue
				}

				wigo.GetLocalWigo().LocalHost.DeleteProbeByName(probeName)
			}
		}

		// Launching probes
		log.Printf("Launching probes of directory %s", directory)

		for c := currentProbesList.Front(); c != nil; c = c.Next() {
			probeName := c.Value.(string)

			if wigo.GetLocalWigo().IsProbeDisabled(probeName) {
				log.Printf(" - Probe %s has been disabled earlier. Restart wigo to enable it again!", probeName)
			} else {
				go execProbe(directory+"/"+probeName, interval, interval-1)
			}
		}

		select {
		case <-stop:
			// Drop the results of a directory that is gone
			for c := currentProbesList.Front(); c != nil; c = c.Next() {
				probeName := c.Value.(string)
				if _, ok := wigo.GetLocalWigo().GetLocalHost().Probes.Get(probeName); ok {
					wigo.GetLocalWigo().GetLocalHost().Probes.Remove(probeName)
				}
			}
			return

		case <-time.After(time.Second * time.Duration(interval)):
		}
	}
}

// reconcileProbesList updates the known probes of a directory in place with
// what is on disk, and returns the names of the ones that disappeared so the
// caller can forget their results.
func reconcileProbesList(directory string, current *list.List, found *list.List) (removed []string) {

	// Check new probes
	for n := found.Front(); n != nil; n = n.Next() {
		newProbeName := n.Value.(string)
		probeIsNew := true

		for c := current.Front(); c != nil; c = c.Next() {
			if c.Value.(string) == newProbeName {
				probeIsNew = false
				break
			}
		}

		if probeIsNew {
			current.PushBack(newProbeName)
			log.Printf("Probe %s has been added in directory %s\n", newProbeName, directory)
		}
	}

	// Check deleted probes. The elements are collected before being removed :
	// removing one while ranging clears its next pointer, which silently ended
	// the loop and left every other deleted probe behind until the next cycle.
	deleted := make([]*list.Element, 0)

	for c := current.Front(); c != nil; c = c.Next() {
		probeName := c.Value.(string)
		probeIsDeleted := true

		for n := found.Front(); n != nil; n = n.Next() {
			if n.Value.(string) == probeName {
				probeIsDeleted = false
				break
			}
		}

		if probeIsDeleted {
			deleted = append(deleted, c)
		}
	}

	removed = make([]string, 0, len(deleted))
	for _, c := range deleted {
		probeName := c.Value.(string)
		log.Printf("Probe %s has been deleted from filesystem.. Removing it from directory.\n", probeName)
		current.Remove(c)
		removed = append(removed, probeName)
	}

	return removed
}

func threadRemoteChecks(remoteWigos []wigo.AdvancedRemoteWigoConfig) {
	log.Println("Listing remoteWigos : ")

	for _, host := range remoteWigos {
		log.Printf(" -> Adding %s to the remote check list\n", host.Hostname)
		go launchRemoteHostCheckRoutine(host)
	}
}

func threadCallbacks(chanCallbacks chan wigo.INotification) {
	httpEnabled := wigo.GetLocalWigo().GetConfig().Notifications.HttpEnabled
	mailEnabled := wigo.GetLocalWigo().GetConfig().Notifications.EmailEnabled
	appriseEnabled := wigo.GetLocalWigo().GetConfig().Notifications.AppriseEnabled

	for {
		notification := <-chanCallbacks

		// Serialize notification
		json, err := notification.ToJson()
		if err != nil {
			log.Printf("Fail to decode notification : %s", err)
			continue
		}

		// Send it
		go func() {
			if httpEnabled != 0 {
				err := wigo.CallbackHttp(string(json))
				if err != nil && mailEnabled == 2 {
					wigo.SendMail(notification.GetSummary(), notification.GetMessage())
				}
			}

			if mailEnabled == 1 {
				wigo.SendMail(notification.GetSummary(), notification.GetMessage())
			}

			if appriseEnabled != 0 {
				wigo.SendApprise(notification)
			}
		}()
	}
}

// execProbe runs a probe once and publishes its result.
//
// The timeout is passed in rather than derived from the interval : a scheduled
// run gets the whole interval minus a second, while one an operator asked for
// is capped, since an http request waits on it.
func execProbe(probePath string, interval int, timeOut int) {

	// Get probe name
	probeDirectory, probeName := path.Split(probePath)

	// Create ProbeResult
	var probeResult *wigo.ProbeResult

	// Every result carries the interval it was produced at, so the API can tell
	// how often a probe runs without reading the probes directory again.
	publish := func(result *wigo.ProbeResult) {
		result.Interval = interval
		wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(result)
	}

	// Stat prob
	fileInfo, err := os.Stat(probePath)
	if err != nil {
		log.Printf("Failed to stat probe %s : %s", probePath, err)
		return
	}

	// Test if executable
	if m := fileInfo.Mode(); m&0111 == 0 {
		log.Printf(" - Probe %s is not executable (%s)", probePath, m.Perm().String())
		return
	}

	// Create Command
	cmd := exec.Command(probePath)

	// Capture stdOut
	commandOutput := make([]byte, 0)

	outputPipe, err := cmd.StdoutPipe()
	if err != nil {
		probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error getting stdout pipe: %s", err), "")
		publish(probeResult)
		return
	}

	errPipe, err := cmd.StderrPipe()
	if err != nil {
		probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error getting stderr pipe: %s", err), "")
		publish(probeResult)
		return
	}

	combinedOutput := io.MultiReader(outputPipe, errPipe)

	// Start
	err = cmd.Start()
	if err != nil {
		probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error starting command: %s", err), "")
		publish(probeResult)
		return
	}

	// Wait channel
	done := make(chan error)
	go func() {
		commandOutput, err = io.ReadAll(combinedOutput)
		if err != nil {
			probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error reading pipe: %s", err), "")
			publish(probeResult)
			return
		}

		done <- cmd.Wait()
	}()

	// Timeout or result ?
	select {
	case err := <-done:
		if err != nil {

			// Get exit code
			exitCode := 1

			if exiterr, ok := err.(*exec.ExitError); ok {
				// The program has exited with an exit code != 0

				// This works on both Unix and Windows. Although package
				// syscall is generally platform dependent, WaitStatus is
				// defined for both Unix and Windows and in both cases has
				// an ExitStatus() method with the same signature.
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
				}
			}

			if exitCode == 12 {
				log.Printf(" - Probe %s is disabled\n", probeName)
				return
			}

			if exitCode == 13 {
				log.Printf(" - Probe %s responded with special exit code 13. Discarding result...\n", probeName)

				// Disabling it
				wigo.GetLocalWigo().DisableProbe(probeName)

				// Remove result if present
				wigo.GetLocalWigo().GetLocalHost().DeleteProbeByName(probeName)

				return
			}

			// Create error probe
			probeResult = wigo.NewProbeResult(probeName, 500, exitCode, fmt.Sprintf("error: %s", err), string(commandOutput))
			publish(probeResult)
			log.Printf(" - Probe %s in directory %s failed to exec : %s - %s\n", probeResult.Name, probeDirectory, err, probeResult.Detail)
			return

		} else {
			probeResult = wigo.NewProbeResultFromJson(probeName, commandOutput)
			publish(probeResult)

			log.Printf(" - Probe %s in directory %s responded with status : %d\n", probeResult.Name, probeDirectory, probeResult.Status)

			if probeResult.Status > 100 {
				log.Printf(" 	--> %s\n", probeResult.Message)
			}
			return
		}

	case <-time.After(time.Second * time.Duration(timeOut)):
		probeResult = wigo.NewProbeResult(probeName, 500, -1, "Probe timeout", "")
		publish(probeResult)

		log.Printf(" - Probe %s in directory %s timeouted..\n", probeResult.Name, probeDirectory)

		// Killing it..
		log.Printf(" - Killing it...")
		err := cmd.Process.Kill()
		if err != nil {
			log.Printf(" - Failed to kill probe %s : %s\n", probeName, err)
			// TODO handle error
		} else {
			log.Printf(" - Probe %s successfully killed\n", probeName)
		}

		return
	}
}

func launchRemoteHostCheckRoutine(Hostname wigo.AdvancedRemoteWigoConfig) {

	secondsToSleep := wigo.GetLocalWigo().GetConfig().RemoteWigos.CheckInterval
	if Hostname.CheckInterval != 0 {
		secondsToSleep = Hostname.CheckInterval
	}

	// Split host/port
	host := Hostname.Hostname
	if Hostname.Port != 0 {
		host = Hostname.Hostname + ":" + strconv.Itoa(Hostname.Port)
	} else {
		host = Hostname.Hostname + ":" + strconv.Itoa(wigo.GetLocalWigo().GetConfig().Http.Port)
	}

	// Create vars
	var resp *http.Response
	var body []byte
	var err error

	// Create http client
	client := http.Client{Timeout: time.Duration(time.Second)}

	sslEnabled := wigo.GetLocalWigo().GetConfig().RemoteWigos.SslEnabled
	if Hostname.SslEnabled {
		sslEnabled = Hostname.SslEnabled
	}

	var protocol string
	if sslEnabled {
		protocol = "https://"
	} else {
		protocol = "http://"
	}
	url := protocol + host + "/api"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("RemoteHostCheckRoutine : Unable to build get request : %s ", err)
		return
	}

	login := wigo.GetLocalWigo().GetConfig().RemoteWigos.Login
	if Hostname.Login != "" {
		login = Hostname.Login
	}

	password := wigo.GetLocalWigo().GetConfig().RemoteWigos.Password
	if Hostname.Password != "" {
		password = Hostname.Password
	}

	if login != "" && password != "" {
		req.SetBasicAuth(login, password)
	}

	for {
		for i := 1; i <= 3; i++ {
			resp, err = client.Do(req)
			if err != nil {
				time.Sleep(time.Second)
			} else {
				body, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				break
			}
		}

		// Can't connect to remote wigo
		if err != nil {
			log.Printf("Can't connect to %s : %s", host, err)
			time.Sleep(time.Second * time.Duration(secondsToSleep))
			continue
		}

		// Instanciate object from remote return
		wigoObj, err := wigo.NewWigoFromJson(body, Hostname.CheckRemotesDepth)
		if err != nil {
			log.Printf("Failed to parse return from host %s : %s\nReturn was : %s", host, err, body)
			time.Sleep(time.Second * time.Duration(secondsToSleep))
			continue
		}

		// Remember where this remote answered, so a call can later be
		// forwarded to it. Only known once it has replied : the configuration
		// holds a network address, while the rest of the API works with the
		// hostname the remote reports for itself, and the two often differ.
		wigo.RegisterRemoteEndpoint(wigoObj.Uuid, protocol+host, login, password)

		// Send it to main
		wigo.GetLocalWigo().AddOrUpdateRemoteWigo(wigoObj)

		// Sleep
		time.Sleep(time.Second * time.Duration(secondsToSleep))
	}
}

func threadHttp(config *wigo.HttpConfig) {
	address := config.Address + ":" + strconv.Itoa(config.Port)

	mux := http.NewServeMux()
	registerRoutes(mux)

	// Read outermost first : a panic is caught before anything else, and the
	// credential is checked before a request reaches a handler or a file.
	middlewares := []wigo.Middleware{wigo.Recovering()}

	if wigo.GetLocalWigo().GetConfig().Global.Debug {
		middlewares = append(middlewares, wigo.Logging())
	}

	middlewares = append(middlewares, wigo.SecurityHeaders())

	if config.Gzip {
		log.Println("Http server : gzip compression enabled")
		middlewares = append(middlewares, wigo.Gzip())
	}

	// Always installed, even with no Login : it is what reads a token, and what
	// decides what somebody presenting nothing is allowed to do.
	anonymousRole := wigo.ResolvedAnonymousRole(config)
	if config.Login != "" && config.Password != "" {
		log.Println("Http server : basic auth enabled")
	}
	switch anonymousRole {
	case wigo.RoleNone:
		log.Println("Http server : credentials required")
	case wigo.RoleOperator:
		log.Println("Http server : open to anyone, as an operator")
	default:
		log.Printf("Http server : open to anyone, %s", anonymousRole)
	}
	middlewares = append(middlewares, wigo.Authenticating(config.Login, config.Password, anonymousRole))

	handler := wigo.Chain(mux, middlewares...)

	// Timeouts, which martini did not set : without them a client that opens a
	// connection and says nothing holds a goroutine for ever. The write one is
	// generous because rechecking a probe on demand waits for it to run.
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if config.SslEnabled {
		log.Println("Http server : starting tls server @ " + address)
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		if err := server.ListenAndServeTLS(config.SslCert, config.SslKey); err != nil {
			log.Fatalf("Failed to start http server : %s", err)
		}
		return
	}

	log.Println("Http server : starting plain http server @ " + address)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start http server : %s", err)
	}
}

// registerRoutes is the whole api in one place.
//
// The patterns are the standard library's since Go 1.22 : a method, a path, and
// {name} for the parts a handler reads with r.PathValue. The most specific
// pattern wins, so /api/hosts and /api/hosts/{hostname} can both be registered
// without ordering them by hand.
func registerRoutes(mux *http.ServeMux) {

	get := func(pattern string, handler wigo.Handler) {
		mux.Handle("GET "+pattern, handler)
	}
	post := func(pattern string, handler wigo.Handler) {
		mux.Handle("POST "+pattern, handler)
	}

	get("/api", func(w http.ResponseWriter, r *http.Request) (int, string) {
		json, err := wigo.GetLocalWigo().ToJsonString()
		if err != nil {
			return 500, fmt.Sprintf("%s", err)
		}
		return 200, json
	})

	get("/api/status", func(w http.ResponseWriter, r *http.Request) (int, string) {
		return 200, strconv.Itoa(wigo.GetLocalWigo().GlobalStatus)
	})

	get("/api/logs", wigo.HttpLogsHandler)
	get("/api/logs/indexes", wigo.HttpLogsIndexesHandler)
	get("/api/groups", wigo.HttpGroupsHandler)
	get("/api/groups/{group}", wigo.HttpGroupsHandler)
	get("/api/groups/{group}/logs", wigo.HttpLogsHandler)
	get("/api/groups/{group}/probes/{probe}/logs", wigo.HttpLogsHandler)
	get("/api/hosts", wigo.HttpRemotesHandler)
	get("/api/hosts/{hostname}", wigo.HttpRemotesHandler)
	get("/api/hosts/{hostname}/status", wigo.HttpRemotesStatusHandler)
	get("/api/hosts/{hostname}/logs", wigo.HttpLogsHandler)
	get("/api/hosts/{hostname}/probes", wigo.HttpRemotesProbesHandler)
	get("/api/hosts/{hostname}/probes/{probe}", wigo.HttpRemotesProbesHandler)
	get("/api/hosts/{hostname}/probes/{probe}/status", wigo.HttpRemotesProbesStatusHandler)
	get("/api/hosts/{hostname}/probes/{probe}/logs", wigo.HttpLogsHandler)
	get("/api/probes/{probe}/logs", wigo.HttpLogsHandler)
	get("/api/probes/{probe}/metrics", wigo.HttpProbeMetricsHandler)
	get("/api/hosts/{hostname}/probes/{probe}/metrics", wigo.HttpHostProbeMetricsHandler)
	get("/api/hosts/{hostname}/timeline", wigo.HttpHostTimelineHandler)

	get("/api/probes", wigo.HttpProbesHandler)
	post("/api/probes/{probe}/disable", wigo.HttpProbeDisableHandler)
	post("/api/probes/{probe}/interval", wigo.HttpProbeIntervalHandler)
	post("/api/probes/{probe}/run", wigo.HttpProbeRunHandler)

	get("/api/hosts/{hostname}/schedule", wigo.HttpHostScheduleHandler)
	post("/api/hosts/{hostname}/probes/{probe}/disable", wigo.HttpHostProbeDisableHandler)
	post("/api/hosts/{hostname}/probes/{probe}/interval", wigo.HttpHostProbeIntervalHandler)
	post("/api/hosts/{hostname}/probes/{probe}/run", wigo.HttpHostProbeRunHandler)

	get("/api/suppressions", wigo.HttpSuppressionsHandler)
	post("/api/hosts/{hostname}/ack", wigo.HttpHostAckHandler)
	post("/api/hosts/{hostname}/silence", wigo.HttpHostSilenceHandler)
	post("/api/hosts/{hostname}/unsuppress", wigo.HttpHostUnsuppressHandler)
	post("/api/groups/{group}/silence", wigo.HttpGroupSilenceHandler)
	post("/api/groups/{group}/unsuppress", wigo.HttpGroupUnsuppressHandler)

	get("/api/whoami", wigo.HttpWhoamiHandler)

	// Navigated to rather than fetched : provoking the browser credential
	// prompt is the whole point, and only a top level navigation does it
	// reliably.
	get("/api/login", wigo.HttpLoginHandler)
	get("/api/logout", wigo.HttpLogoutHandler)
	get("/api/tokens", wigo.HttpTokensHandler)
	post("/api/tokens", wigo.HttpTokenCreateHandler)
	post("/api/tokens/{id}/revoke", wigo.HttpTokenRevokeHandler)

	get("/api/authority/hosts", wigo.HttpAuthorityListHandler)
	post("/api/authority/hosts/{uuid}/allow", wigo.HttpAuthorityAllowHandler)
	post("/api/authority/hosts/{uuid}/revoke", wigo.HttpAuthorityRevokeHandler)

	// Not a wigo.Handler : a stream has no status and body to return
	mux.Handle("GET /api/events", http.HandlerFunc(wigo.HttpEventsHandler))

	// Outside /api on purpose : /metrics is where every scraper looks
	get("/metrics", wigo.HttpMetricsHandler)

	// Everything else is the built interface. Registered last and on the bare
	// root, so it only ever sees what no route above claimed.
	mux.Handle("/", wigo.StaticFiles("public"))
}

func threadPush(config *wigo.PushClientConfig) {
	var pushClient *wigo.PushClient
	go func() {
		for {
			var err error

			if pushClient == nil {
				pushClient, err = wigo.NewPushClient(config)
				if err == nil {
					err = pushClient.Hello()
					if err != nil {
						pushClient.Close()
						pushClient = nil
						if err.Error() != "RECONNECT" {
							time.Sleep(time.Duration(config.PushInterval) * time.Second)
						}
						continue
					}
				} else {
					pushClient.Close()
					pushClient = nil
					if err.Error() != "RECONNECT" {
						time.Sleep(time.Duration(config.PushInterval) * time.Second)
					}
					continue
				}
			}

			time.Sleep(time.Duration(config.PushInterval) * time.Second)
			err = pushClient.Update()
			if err != nil {
				pushClient.Close()
				pushClient = nil
				continue
			}

			// Ask for our orders on the connection we already hold : the server
			// cannot reach us, so nothing is ever pushed at us. Does nothing
			// unless this client was configured to accept being driven.
			if err = pushClient.PollCommands(); err != nil {
				pushClient.Close()
				pushClient = nil
			}
		}
	}()
}
