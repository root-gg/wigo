package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
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
)


func main() {

	// Init Wigo
	wigo.InitWigo()


	// Launch goroutines
	go threadWatch(wigo.Channels.ChanWatch)
	go threadLocalChecks()
	go threadRemoteChecks(wigo.GetLocalWigo().GetConfig().HostsToCheck)
	go threadSocket(wigo.GetLocalWigo().GetConfig().ListenAddress, wigo.GetLocalWigo().GetConfig().ListenPort)
	go threadCallbacks(wigo.Channels.ChanCallbacks)


	// Signals
	signal.Notify(wigo.Channels.ChanSignals, syscall.SIGINT, syscall.SIGTERM)


	// Selection
	for {
		select {

		case <-wigo.Channels.ChanSignals :
			os.Exit(0)
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
	}

	// Create a watcher on checks directory
	err = watcherNew.Watch(wigo.GetLocalWigo().GetConfig().ProbesDirectory)
	if err != nil {
		log.Fatal(err)
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
								continue
							}
						}

						// Launching probes
						log.Printf("Launching probes of directory %s", directory)

						for c := currentProbesList.Front(); c != nil; c = c.Next() {
							probeName := c.Value.(string)

							go execProbe(directory+"/"+probeName, 5)
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

func threadRemoteChecks(hostsToCheck []string) {
	log.Println("Listing hostsToCheck : ")

	for _, host := range hostsToCheck {
		log.Printf(" -> Adding %s to the remote check list\n", host)
		go launchRemoteHostCheckRoutine(host)
	}
}

func threadSocket(listenAddress string, listenPort int) {

	// Listen
	listener, err := net.Listen("tcp4", listenAddress+":"+strconv.Itoa(listenPort))
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	// Serve
	for {
		c, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go func(){
			log.Printf("Got a new connection : %s\n",c.RemoteAddr())
			defer c.Close()

			json,err := wigo.GetLocalWigo().ToJsonString()
			if(err != nil){
				log.Println("Fail to encode to json : ", err)
				return
			}

			// Print json to socket
			fmt.Fprintln(c, json)

			return
		}()
	}
}

func threadCallbacks(chanCallbacks chan wigo.INotification) {
	for {
		notification := <-chanCallbacks
		callbackUrl  := wigo.GetLocalWigo().GetConfig().CallbackUrl

		if callbackUrl != "" {
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

				// Jsonize notification
				json, err := notification.ToJson()
				if err != nil {
					return
				}

				// Make post values
				postValues := url.Values{}
				postValues.Add("Notification", string(json))


				// Make request
				_, reqErr := c.PostForm( callbackUrl, postValues )
				if (reqErr != nil) {
					log.Printf("Error sending callback to url %s : %s", notification.GetReceiver(), reqErr)
				} else {
					log.Printf("Notif : %s", notification.GetMessage())
					//log.Printf("Successfully called callback url %s", notification.GetReceiver())
				}
			}()
		}
	}
}


func execProbe(probePath string, timeOut int) {

	// Get probe name
	probeDirectory , probeName := path.Split(probePath)

	// Create ProbeResult
	var probeResult *wigo.ProbeResult

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
			probeResult = wigo.NewProbeResult(probeName, 500, -1, fmt.Sprintf("error: %s", err), string(commandOutput))
			wigo.GetLocalWigo().GetLocalHost().AddOrUpdateProbe(probeResult)
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

func launchRemoteHostCheckRoutine(host string) {
	for {

		conn, err := net.DialTimeout("tcp", host, time.Second * 2)

		if err != nil {
			log.Printf("Error connecting to host %s : %s", host, err)

			// Create wigo in error
			errorWigo := wigo.NewWigoFromErrorMessage(fmt.Sprint(err), false)
			errorWigo.SetHostname(host)

			wigo.GetLocalWigo().AddOrUpdateRemoteWigo( host, errorWigo)

		} else {

			completeOutput := new(bytes.Buffer)

			for {
				reply := make([]byte, 512)
				read_len, err := conn.Read(reply)
				if ( err != nil ) {
					break
				}

				completeOutput.Write(reply[:read_len])
			}

			// Instanciate object from remote return
			wigoObj, err := wigo.NewWigoFromJson(completeOutput.Bytes())
			if (err != nil) {
				log.Printf("Failed to parse return from host %s : %s", host, err)
				continue
			}

			// Set hostname with config file name
			wigoObj.SetHostname(host)

			// Send it to main
			wigo.GetLocalWigo().AddOrUpdateRemoteWigo( host, wigoObj)
		}

		time.Sleep(time.Second * 10)
	}
}

