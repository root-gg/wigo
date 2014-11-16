package main

import (
	_ "net/http/pprof"
	"strings"
	"crypto/tls"
	"container/list"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"syscall"
	"time"
    "net/http"

	"wigo"

	"github.com/codegangsta/martini"
	"github.com/codegangsta/martini-contrib/auth"
	"github.com/codegangsta/martini-contrib/gzip"
	"github.com/codegangsta/martini-contrib/secure"

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

	if ( config.Global.Debug ) {
		// Debug heap
		go func() {
			log.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	// Loads previous logs
	go wigo.LoadLogsFromDisk()

	// Launch goroutines
	go threadWatch(wigo.Channels.ChanWatch)
	go threadLocalChecks()

	go threadCallbacks(wigo.Channels.ChanCallbacks)
	go threadRemoteChecks(config.RemoteWigos.AdvancedList)
	go threadAliveChecks()

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
		ci <- wigo.Event{wigo.ADDDIRECTORY, wigo.GetLocalWigo().GetConfig().Global.ProbesDirectory + "/" + dir}
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
					ci <- wigo.Event{wigo.ADDDIRECTORY, ev.Name}
				}

			} else if ev.IsDelete() {
				ci <- wigo.Event{wigo.REMOVEDIRECTORY, ev.Name}
			} else if ev.IsRename() {
				ci <- wigo.Event{wigo.REMOVEDIRECTORY, ev.Name}
			}
		}
	}
}

func threadLocalChecks() {

	// Directory list
	var checksDirectories list.List

	// Listen events
	go func() {
		for {
			ev := <-wigo.Channels.ChanWatch

			switch ev.Type {
			case wigo.ADDDIRECTORY:

				var directory string = ev.Value.(string)

				log.Println("Adding directory", directory)
				checksDirectories.PushBack(directory)

				// Create local list of probes to detect removes
				currentProbesList, err := wigo.ListProbesInDirectory(directory)
				if err != nil {
					log.Printf("Fail to read directory %s : %s", directory, err)
				}

				go func() {
					for {

						// Am I still a valid directory ?
						stillValid := false
						for e := checksDirectories.Front(); e != nil; e = e.Next() {
							if e.Value == directory {
								stillValid = true
							}
						}
						if !stillValid {

							// Delete probes results of this directory
							for c := currentProbesList.Front(); c != nil; c = c.Next() {
								probeName := c.Value.(string)
								if _, ok := wigo.GetLocalWigo().GetLocalHost().Probes[probeName]; ok {
									delete(wigo.GetLocalWigo().GetLocalHost().Probes, probeName)
								}
							}

							return
						}

						// Guess sleep time from dir
						sleepTime := path.Base(directory)
						sleepTImeInt, err := strconv.Atoi(sleepTime)
						if err != nil {
							log.Printf(" - Weird folder name %s. Doing nothing...\n", directory)
							return
						}

						// Update probes list
						newProbesList, err := wigo.ListProbesInDirectory(directory)
						if err != nil {
							break
						}

						// Check new probes
						for n := newProbesList.Front(); n != nil; n = n.Next() {
							newProbeName := n.Value.(string)
							probeIsNew := true

							// Add probe if new
							for j := currentProbesList.Front(); j != nil; j = j.Next() {
								probeName := j.Value.(string)

								if probeName == newProbeName {
									probeIsNew = false
								}
							}

							if probeIsNew {
								currentProbesList.PushBack(newProbeName)
								log.Printf("Probe %s has been added in directory %s\n", newProbeName, directory)
							}
						}

						// Check deleted probes
						for c := currentProbesList.Front(); c != nil; c = c.Next() {
							probeName := c.Value.(string)
							probeIsDeleted := true

							for n := newProbesList.Front(); n != nil; n = n.Next() {
								newProbeName := n.Value.(string)

								if probeName == newProbeName {
									probeIsDeleted = false
								}
							}

							if probeIsDeleted {
								log.Printf("Probe %s has been deleted from filesystem.. Removing it from directory.\n", probeName)
								currentProbesList.Remove(c)
								wigo.GetLocalWigo().LocalHost.DeleteProbeByName(probeName)
								continue
							}
						}

						// Launching probes
						log.Printf("Launching probes of directory %s", directory)

						for c := currentProbesList.Front(); c != nil; c = c.Next() {
							probeName := c.Value.(string)

							if wigo.GetLocalWigo().IsProbeDisabled(probeName) {
								log.Printf(" - Probe %s has been disabled earlier. Restart wigo to enable it again!", probeName)
							} else {
								go execProbe(directory+"/"+probeName, sleepTImeInt-1)
							}
						}

						time.Sleep(time.Second * time.Duration(sleepTImeInt))
					}
				}()

			case wigo.REMOVEDIRECTORY:
				for el := checksDirectories.Front(); el != nil; el = el.Next() {
					if el.Value == ev.Value {
						log.Println("Removing directory ", ev.Value)
						checksDirectories.Remove(el)
						break
					}
				}
			}
		}
	}()
}

func threadRemoteChecks(remoteWigos []wigo.AdvancedRemoteWigoConfig) {
	log.Println("Listing remoteWigos : ")

	for _, host := range remoteWigos {
		log.Printf(" -> Adding %s to the remote check list\n", host.Hostname)
		go launchRemoteHostCheckRoutine(host)
	}
}

func threadAliveChecks() {
	for {
		now := time.Now().Unix()
		for _, host := range wigo.GetLocalWigo().RemoteWigos {
			if host.LastUpdate < now - int64(wigo.GetLocalWigo().GetConfig().Global.AliveTimeout) {
				if ( host.IsAlive ) {
					message := fmt.Sprintf("Wigo %s DOWN", host.GetHostname())
					wigo.SendNotification(wigo.NewNotificationFromMessage(message))
					wigo.GetLocalWigo().AddLog(host, wigo.CRITICAL, message)
				}
				host.IsAlive = false;
				host.GlobalStatus = 499;
			} else {
				if ( !host.IsAlive ) {
					message := fmt.Sprintf("Wigo %s UP", host.GetHostname())
					wigo.SendNotification(wigo.NewNotificationFromMessage(message))
					wigo.GetLocalWigo().AddLog(host, wigo.INFO, message)
				}
				host.IsAlive = true;
			}
		}
		time.Sleep(time.Second)
	}
}

func threadCallbacks(chanCallbacks chan wigo.INotification) {
	httpEnabled := wigo.GetLocalWigo().GetConfig().Notifications.HttpEnabled
	mailEnabled := wigo.GetLocalWigo().GetConfig().Notifications.EmailEnabled

	for {
		notification := <-chanCallbacks

		// Serialize notification
		json, err := notification.ToJson()
		if err != nil {
			log.Printf("Fail to decode notification : ", err)
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
		}()
	}
}

func execProbe(probePath string, timeOut int) {

	// Get probe name
	probeDirectory, probeName := path.Split(probePath)

	// Create ProbeResult
	var probeResult *wigo.ProbeResult

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
		wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)
		return
	}

	errPipe, err := cmd.StderrPipe()
	if err != nil {
		probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error getting stderr pipe: %s", err), "")
		wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)
		return
	}

	combinedOutput := io.MultiReader(outputPipe, errPipe)

	// Start
	err = cmd.Start()
	if err != nil {
		probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error starting command: %s", err), "")
		wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)
		return
	}

	// Wait channel
	done := make(chan error)
	go func() {
		commandOutput, err = ioutil.ReadAll(combinedOutput)
		if err != nil {
			probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error reading pipe: %s", err), "")
			wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)
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

			// Test Exit 13
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
			wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)

			log.Printf(" - Probe %s in directory %s failed to exec : %s\n", probeResult.Name, probeDirectory, err)
			return

		} else {
			probeResult = wigo.NewProbeResultFromJson(probeName, commandOutput)
			wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)

			log.Printf(" - Probe %s in directory %s responded with status : %d\n", probeResult.Name, probeDirectory, probeResult.Status)

			if probeResult.Status > 100 {
				log.Printf(" 	--> %s\n", probeResult.Message)
			}
			return
		}

	case <-time.After(time.Second * time.Duration(timeOut)):
		probeResult = wigo.NewProbeResult(probeName, 500, -1, "Probe timeout", "")
		wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)

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
		host = Hostname.Hostname + ":" + strconv.Itoa(wigo.GetLocalWigo().GetConfig().Global.ListenPort)
	}

	// Create vars
	var resp *http.Response
	var body []byte
	var err error

	// Create http client
	client := http.Client{Timeout: time.Duration(time.Second)}

	// Try
	tries := wigo.GetLocalWigo().GetConfig().RemoteWigos.CheckTries
	if Hostname.CheckTries != 0 {
		tries = Hostname.CheckTries
	}

	// TODO handle ssl and basic auth
	var protocol string
	if Hostname.SslEnabled {
		protocol = "https://"
	} else {
		protocol = "http://"
	}
	url := protocol + host + "/api"

	req, err := http.NewRequest("GET",url, nil)
	if err != nil {
		log.Printf("RemoteHostCheckRoutine : Unable to build get request : %s ", err)
		return
	}

	if Hostname.Login != "" && Hostname.Password != "" {
		req.SetBasicAuth("<username>", "<password>")
	}

	for {
		for i := 1; i <= tries; i++ {
			resp, err = client.Do(req)
			if err != nil {
				time.Sleep(time.Second)
			} else {
				body, _ = ioutil.ReadAll(resp.Body)
                resp.Body.Close()
				break
			}
		}

		// Can't connect, give up and create wigo in error
		if err != nil {
			log.Printf("Can't connect to %s after %d tries : %s", host, tries, err)

			// Create wigo in error
			if existingWigo, ok := wigo.GetLocalWigo().RemoteWigos[Hostname.Hostname] ; ok {
				newWigo := *existingWigo
				newWigo.Down(fmt.Sprintf("%s",err))
				wigo.GetLocalWigo().AddOrUpdateRemoteWigo(&newWigo)
			} else {
				errorWigo := wigo.NewWigoFromErrorMessage(fmt.Sprint(err), false)
				errorWigo.SetHostname(Hostname.Hostname)
				wigo.GetLocalWigo().AddOrUpdateRemoteWigo(errorWigo)
			}

			time.Sleep(time.Second * time.Duration(secondsToSleep))
			continue
		}

		// Instanciate object from remote return
		wigoObj, err := wigo.NewWigoFromJson(body, Hostname.CheckRemotesDepth)
		if err != nil {
			log.Printf("Failed to parse return from host %s : %s", host, err)
			time.Sleep(time.Second * time.Duration(secondsToSleep))
			continue
		}

		// Set hostname with config file name
		//wigoObj.SetHostname(Hostname.Hostname)

		// Send it to main
		wigo.GetLocalWigo().AddOrUpdateRemoteWigo(wigoObj)

		// Sleep
		time.Sleep(time.Second * time.Duration(secondsToSleep))
	}
}

func threadHttp(config *wigo.HttpConfig) {
	apiAddress := config.Address
	apiPort := config.Port

	m := martini.New()

	if ( wigo.GetLocalWigo().GetConfig().Global.Debug ) {
		// Log requests
		m.Use(martini.Logger())
	}

	// Compress http responses with gzip
	if ( config.Gzip ) {
		log.Println("Http server : gzip compression enabled")
		m.Use(gzip.All())
	}

	// Add some basic security checks
	m.Use(secure.Secure(secure.Options{}));

	// Http basic auth
	if ( config.Login != "" && config.Password != "" ) {
		log.Println("Http server : basic auth enabled")
		m.Use(auth.Basic(config.Login, config.Password))
	}

	// Serve static files
	m.Use(martini.Static("public"))

	// Handle errors // TODO is this even working ?
	m.Use(martini.Recovery())

	// Define the routes.

	r := martini.NewRouter()

	r.Get("/api", func() (int, string) {
		json, err := wigo.GetLocalWigo().ToJsonString()
		if err != nil {
			return 500, fmt.Sprintf("%s", err)
		}
		return 200, json
	})

	r.Get("/api/status", func() string { return strconv.Itoa((wigo.GetLocalWigo().GlobalStatus)) })
	r.Get("/api/logs", wigo.HttpLogsHandler)
	r.Get("/api/logs/indexes", wigo.HttpLogsIndexesHandler)
	r.Get("/api/groups", wigo.HttpGroupsHandler)
	r.Get("/api/groups/:group", wigo.HttpGroupsHandler)
	r.Get("/api/groups/:group/logs",wigo.HttpLogsHandler)
	r.Get("/api/groups/:group/probes/:probe/logs",wigo.HttpLogsHandler)
	r.Get("/api/hosts", wigo.HttpRemotesHandler)
	r.Get("/api/hosts/:hostname", wigo.HttpRemotesHandler)
	r.Get("/api/hosts/:hostname/status", wigo.HttpRemotesStatusHandler)
	r.Get("/api/hosts/:hostname/logs", wigo.HttpLogsHandler)
	r.Get("/api/hosts/:hostname/probes", wigo.HttpRemotesProbesHandler)
	r.Get("/api/hosts/:hostname/probes/:probe", wigo.HttpRemotesProbesHandler)
	r.Get("/api/hosts/:hostname/probes/:probe/status", wigo.HttpRemotesProbesStatusHandler)
	r.Get("/api/hosts/:hostname/probes/:probe/logs", wigo.HttpLogsHandler)
	r.Get("/api/probes/:probe/logs", wigo.HttpLogsHandler)
	r.Get("/api/authority/hosts", wigo.HttpAuthorityListHandler)
	r.Post("/api/authority/hosts/:uuid/allow",wigo.HttpAuthorityAllowHandler)
	r.Post("/api/authority/hosts/:uuid/revoke",wigo.HttpAuthorityRevokeHandler)

	m.Use(func(c martini.Context, w http.ResponseWriter, r *http.Request) {
		if ( strings.HasPrefix(r.URL.Path,"/api") ) {
			w.Header().Set("Content-Type", "application/json")
		}
	})

	m.Action(r.Handle)

	// Create a listner and serv connections forever.
	if ( config.SslEnabled ) {
		address := apiAddress + ":" + strconv.Itoa(apiPort)
		log.Println("Http server : starting tls server @ " + address)
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		server := &http.Server{Addr: address, Handler: m, TLSConfig: tlsConfig}
		err := server.ListenAndServeTLS(config.SslCert, config.SslKey)
		if err != nil {
			log.Fatalf("Failed to start http server : %s", err)
		}
	} else {
		address := apiAddress + ":" + strconv.Itoa(apiPort)
		log.Println("Http server : starting plain http server @ " + address)
		if err := http.ListenAndServe(address, m) ; err != nil {
			log.Fatalf("Failed to start http server : %s", err)
		}
	}
}

func threadPush(config *wigo.PushClientConfig) {
	var pushClient *wigo.PushClient
	go func(){
		for {
			var err error

			if ( pushClient == nil ) {
				pushClient, err = wigo.NewPushClient(config)
				if ( err == nil ) {
					err = pushClient.Hello()
					if ( err != nil ) {
						pushClient.Close()
						pushClient = nil
						if ( err.Error() != "RECONNECT" ) {
							time.Sleep(time.Duration(config.PushInterval) * time.Second)
						}
						continue
					}
				} else {
					pushClient.Close()
					pushClient = nil
					if ( err.Error() != "RECONNECT" ) {
						time.Sleep(time.Duration(config.PushInterval) * time.Second)
					}
					continue
				}
			}

			time.Sleep(time.Duration(config.PushInterval) * time.Second)
			err = pushClient.Update()
			if ( err != nil ) {
				pushClient.Close()
				pushClient = nil
			}
		}
	}()
}
