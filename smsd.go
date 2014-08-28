package main

import (
    "fmt"
    "log"
    "net"
    "os"
    "os/exec"
    "io/ioutil"
    "time"
)

const listenAddr        = "localhost:4000"
const checksDirectory   = "/home/mbodjiki/checks"


func main() {

    // Channels
    chanChecks := make(chan string)
    chanSocket := make(chan string)

    // Launch goroutines
    go threadChecks(chanChecks)
    go threadSocket(chanSocket)

    // Selection
    for{
        select {
            case msg := <-chanChecks :
                fmt.Printf("[MAIN  ] Received something from cheks channel : %s\n" , msg)

            case msg := <-chanSocket :
                fmt.Printf("[MAIN  ] Received something from socket channel : %s\n" , msg)
                fmt.Printf("[MAIN  ] -> Sending json\n")
                chanSocket <- "lol"
        }
    }
}


//
//// Threads
//

func threadChecks( ci chan string ) {
    for {
        ci <- "thread-checks-keepalive"
        time.Sleep( time.Minute )
    }
}
func threadSocket( ci chan string ) {

    // Listen
    listener, err := net.Listen("tcp", listenAddr)
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

        ci <- "thread-socket-newConnection"
        msg := <-ci

        fmt.Printf("[SOCKET] Received msg from main thread : %s \n",msg)

        go handleRequest(c)
    }
}




func handleRequest( c net.Conn ) error {
    checkOutput, err := runCheck("/home/mbodjiki/hardware_load_avg")

    myHostname, err := os.Hostname()

    if err != nil {
        fmt.Fprintf(c,"Error : %s",err  )
    } else {
        fmt.Fprintln(c,myHostname)
        fmt.Fprintln(c,checkOutput)

        checkDirectories, err := listChecksDirectories()
        if err != nil {
        }

        for _ , directory := range checkDirectories {
            fmt.Fprintln(c, "Got a directory " + directory )
        }

    }
    return c.Close()
}


func runCheck( path string ) (string,error) {
    out, err := exec.Command( path ).Output()
    return string(out),err
}

func listChecksDirectories() ([]string,error) {

    // List checks directory
    files, err := ioutil.ReadDir(checksDirectory)
    if err != nil {
        return nil, err
    }

    // Init array
    subdirectories := make([]string, 0)


    // Return only subdirectories
    for _, f := range files {
        if(f.IsDir()){
            subdirectories = append( subdirectories, f.Name())
        }
    }

    return subdirectories, nil
}
