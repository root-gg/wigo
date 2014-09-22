package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"net/smtp"
	"net/mail"
	"os"
	"os/exec"
	"os/signal"
	"io"
	"io/ioutil"
	"time"
	"path"
	"syscall"
	"strconv"
	"container/list"
	"bytes"
	"crypto/tls"

	// Custom libs
	"wigo"
	"github.com/howeyc/fsnotify"
	"github.com/codegangsta/martini"
)


func main() {

	// Init Wigo
	err := wigo.InitWigo()
	if err != nil {
		log.Printf("Error initialising Wigo : %s", err)
		os.Exit(1)
	}


	// Launch goroutines
	go threadWatch(wigo.Channels.ChanWatch)
	go threadLocalChecks()
	go threadRemoteChecks(wigo.GetLocalWigo().GetConfig().AdvancedRemoteWigosList)
	go threadCallbacks(wigo.Channels.ChanCallbacks)
	go threadHttp()

	// Signals
	signal.Notify(wigo.Channels.ChanSignals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)


	// Selection
	for {
		select {

		case sig := <-wigo.Channels.ChanSignals :
			switch sig {
			case syscall.SIGHUP :
				log.Printf("Caught SIGHUP. Reloading logger filehandle and configuration file...\n")
				wigo.GetLocalWigo().InitOrReloadLogger()
			case syscall.SIGTERM :
                os.Exit(0)
			case os.Interrupt :
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
		ci <- wigo.Event{ wigo.ADDDIRECTORY, wigo.GetLocalWigo().GetConfig().ProbesDirectory + "/" + dir }
	}

	// Init inotify
	watcherNew, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
		return
	}

	// Create a watcher on checks directory
	err = watcherNew.Watch(wigo.GetLocalWigo().GetConfig().ProbesDirectory)
	if err != nil {
		log.Fatal(err)
		return
	}

	// Watch for changes forever
	for {
		select {

		case ev := <- watcherNew.Event:

			if ev.IsCreate() {
				fileInfo, err := os.Stat(ev.Name)
				if err != nil{
					log.Printf("Error stating %s : %s", ev.Name, err)
					return
				}

				if fileInfo.IsDir() {
					ci <- wigo.Event{ wigo.ADDDIRECTORY, ev.Name}
				}

			} else if ev.IsDelete() {
				ci <- wigo.Event{ wigo.REMOVEDIRECTORY, ev.Name}
			} else if ev.IsRename() {
				ci <- wigo.Event{ wigo.REMOVEDIRECTORY, ev.Name}
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
			case wigo.ADDDIRECTORY :

				var directory string = ev.Value.(string)

				log.Println("Adding directory", directory)
				checksDirectories.PushBack(directory)

				// Create local list of probes to detect removes
				currentProbesList, err := wigo.ListProbesInDirectory(directory)
				if (err != nil) {
					log.Printf("Fail to read directory %s : %s", directory, err)
				}

				go func() {
					for {

						// Am I still a valid directory ?
						stillValid := false
						for e := checksDirectories.Front(); e != nil; e = e.Next() {
							if (e.Value == directory) {
								stillValid = true
							}
						}
						if (!stillValid) {

							// Delete probes results of this directory
							for c := currentProbesList.Front(); c != nil; c = c.Next() {
								probeName := c.Value.(string)
								if _,ok := wigo.GetLocalWigo().GetLocalHost().Probes[probeName] ; ok {
									delete(wigo.GetLocalWigo().GetLocalHost().Probes, probeName)
								}
							}

							return
						}

						// Guess sleep time from dir
						sleepTime := path.Base(directory)
						sleepTImeInt, err := strconv.Atoi(sleepTime)
						if (err != nil) {
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

								if (probeName == newProbeName) {
									probeIsNew = false
								}
							}

							if (probeIsNew) {
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

								if (probeName == newProbeName) {
									probeIsDeleted = false
								}
							}

							if (probeIsDeleted) {
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

							if wigo.GetLocalWigo().IsProbeDisabled( probeName ) {
								log.Printf(" - Probe %s has been disabled earlier. Restart wigo to enable it again!", probeName )
							} else {
								go execProbe(directory+"/"+probeName, sleepTImeInt - 1)
							}
						}

						// Sleep right amount of time
						time.Sleep(time.Second * time.Duration(sleepTImeInt))
					}
				}()

			case wigo.REMOVEDIRECTORY :
				for el := checksDirectories.Front(); el != nil ; el = el.Next() {
					if (el.Value == ev.Value) {
						log.Println("Removing directory ", ev.Value)
						checksDirectories.Remove(el)
						break
					}
				}
			}
		}
	}()
}

func threadRemoteChecks(remoteWigos []wigo.RemoteWigoConfig) {
	log.Println("Listing remoteWigos : ")

	for _, host := range remoteWigos {
		log.Printf(" -> Adding %s to the remote check list\n", host.Hostname)
		go launchRemoteHostCheckRoutine(host)
	}
}

func threadCallbacks(chanCallbacks chan wigo.INotification) {
	httpEnabled := wigo.GetLocalWigo().GetConfig().NotificationsHttpEnabled
	mailEnabled := wigo.GetLocalWigo().GetConfig().NotificationsEmailEnabled

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
			if httpEnabled {

				httpUrl := wigo.GetLocalWigo().GetConfig().NotificationsHttpUrl

				go func() {
					// Create http client with timeout
					c := http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
							Dial: func(netw, addr string) (net.Conn, error) {
								deadline := time.Now().Add(2 * time.Second)
								c, err := net.DialTimeout(netw, addr, time.Second*2)
								if err != nil {
									return nil, err
								}
								c.SetDeadline(deadline)
								return c, nil
							},
						},
					}

					// Make post values
					postValues := url.Values{}
					postValues.Add("Notification", string(json))

					// Make request
					_, reqErr := c.PostForm(httpUrl, postValues)
					if (reqErr != nil) {
						log.Printf("Error sending callback to url %s : %s", httpUrl, reqErr)
					} else {
						log.Printf(" - Sent to http url : %s", httpUrl)
					}
				}()
			}

			if mailEnabled {

				recipients := wigo.GetLocalWigo().GetConfig().NotificationsEmailRecipients
				server := wigo.GetLocalWigo().GetConfig().NotificationsEmailSmtpServer
				from := mail.Address{
					wigo.GetLocalWigo().GetConfig().NotificationsEmailFromName,
					wigo.GetLocalWigo().GetConfig().NotificationsEmailFromAddress,
				}


				for i := range recipients {

					to := mail.Address{ "", recipients[i] }

					go func() {
						// setup a map for the headers
						header := make(map[string]string)
						header["From"] = from.String()
						header["To"] = to.String()
						header["Subject"] = notification.GetMessage()

						// setup the message
						message := ""
						for k, v := range header {
							message += fmt.Sprintf("%s: %s\r\n", k, v)
						}
						message += "\r\n"
						message += notification.GetSummary()


						// Connect to the remote SMTP server.
						c, err := smtp.Dial(server)
						if err != nil {
							log.Printf("Fail to dial connect to smtp server %s : %s", server, err)
							return
						}


						// Set the sender and recipient.
						c.Mail(from.Address)
						c.Rcpt(to.Address)


						// Send the email body.
						wc, err := c.Data()
						if err != nil {
							log.Printf("Fail to send DATA to smtp server : %s", err)
							return
						}

						buf := bytes.NewBufferString(message)
						if _, err = buf.WriteTo(wc); err != nil {
							log.Printf("Fail to send notification to %s : %s", to.String(), err)
							return
						}

						log.Printf(" - Sent to email address %s", to.String())

						wc.Close()
					}()
				}
			}
		}()
	}
}


func execProbe(probePath string, timeOut int) {

	// Get probe name
	probeDirectory , probeName := path.Split(probePath)

	// Create ProbeResult
	var probeResult *wigo.ProbeResult

	// Stat prob
	fileInfo, err := os.Stat(probePath)
	if err != nil {
		log.Printf("Failed to stat probe %s : %s",probePath,err)
		return
	}

	// Test if executable
	if m := fileInfo.Mode() ; m&0111 == 0 {
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
		if (err != nil) {
			probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error reading pipe: %s", err), "")
			wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)
			return
		}

		done <-cmd.Wait()
	}()


	// Timeout or result ?
	select {
	case err := <-done :
		if (err != nil) {

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
				wigo.GetLocalWigo().DisableProbe( probeName )

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
			return
		}

	case <-time.After(time.Second * time.Duration(timeOut)) :
		probeResult = wigo.NewProbeResult(probeName, 500, -1, "Probe timeout", "")
		wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)

		log.Printf(" - Probe %s in directory %s timeouted..\n", probeResult.Name, probeDirectory)

		// Killing it..
		log.Printf(" - Killing it...")
		err := cmd.Process.Kill()
		if (err != nil) {
			log.Printf(" - Failed to kill probe %s : %s\n", probeName, err)
			// TODO handle error
		} else {
			log.Printf(" - Probe %s successfully killed\n", probeName)
		}

		return
	}
}

func launchRemoteHostCheckRoutine(Hostname wigo.RemoteWigoConfig) {

    secondsToSleep := wigo.GetLocalWigo().GetConfig().RemoteWigosCheckInterval
    if Hostname.CheckInterval != 0 {
        secondsToSleep = Hostname.CheckInterval
    }

	// Split host/port
	host := Hostname.Hostname
    if Hostname.Port != 0 {
        host = Hostname.Hostname + ":" + strconv.Itoa(Hostname.Port)
    } else {
        host = Hostname.Hostname + ":" + strconv.Itoa(wigo.GetLocalWigo().GetConfig().ListenPort)
    }

	for {

		// Create vars
		var resp *http.Response
		var body []byte
		var err error

		// Create http client
		client := http.Client{ Timeout: time.Duration( time.Second ) }

		// Try
        tries := wigo.GetLocalWigo().GetConfig().RemoteWigosCheckTries
        if Hostname.CheckTries != 0 {
            tries = Hostname.CheckTries
        }

		for i := 1; i <= tries; i++ {
			resp, err = client.Get("http://" + host)
			if err != nil {
				time.Sleep(time.Second)
			} else {
				defer resp.Body.Close()
				body, _ = ioutil.ReadAll(resp.Body)
				break
			}
		}

		// Can't connect, give up and create wigo in error
		if err != nil {
			log.Printf("Can't connect to %s after %d tries : %s", host, tries, err)

			// Create wigo in error
			errorWigo := wigo.NewWigoFromErrorMessage(fmt.Sprint(err), false)
			errorWigo.SetHostname(host)

			wigo.GetLocalWigo().AddOrUpdateRemoteWigo(host, errorWigo)

			time.Sleep( time.Second * time.Duration(secondsToSleep) )
			continue
		}

		// Instanciate object from remote return
		wigoObj, err := wigo.NewWigoFromJson(body, Hostname.CheckRemotesDepth)
		if (err != nil) {
			log.Printf("Failed to parse return from host %s : %s", host, err)
			time.Sleep( time.Second * time.Duration(secondsToSleep))
			continue
		}

		// Set hostname with config file name
		wigoObj.SetHostname(host)

		// Send it to main
		wigo.GetLocalWigo().AddOrUpdateRemoteWigo( host, wigoObj)

		// Sleep
		time.Sleep( time.Second * time.Duration(secondsToSleep))
	}
}

func threadHttp(){

	apiAddress 	:= wigo.GetLocalWigo().GetConfig().ListenAddress
	apiPort 	:= wigo.GetLocalWigo().GetConfig().ListenPort

	m := martini.Classic()
	m.Get("/", func() (int, string) {
		json, err := wigo.GetLocalWigo().ToJsonString()
		if err != nil {
			return 500,fmt.Sprintf("%s", err)
		}
		return 200,json
	})

	m.Get("/status", func() string { return strconv.Itoa((wigo.GetLocalWigo().GlobalStatus))})
	m.Get("/remotes", wigo.HttpRemotesHandler)
	m.Get("/remotes/:hostname", wigo.HttpRemotesHandler)
	m.Get("/remotes/:hostname/status", wigo.HttpRemotesStatusHandler)
	m.Get("/remotes/:hostname/probes", wigo.HttpRemotesProbesHandler)
	m.Get("/remotes/:hostname/probes/:probe", wigo.HttpRemotesProbesHandler)
	m.Get("/remotes/:hostname/probes/:probe/status", wigo.HttpRemotesProbesStatusHandler)

	err := http.ListenAndServe( apiAddress + ":" + strconv.Itoa(apiPort), m)
	if err != nil {
		log.Fatalf("Failed to spawn API : %s", err)
	}
}
