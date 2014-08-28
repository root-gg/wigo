package main

import (
    "fmt"
    "log"
    "net"
    "os"
    "os/exec"
    "io/ioutil"
)

const listenAddr        = "localhost:4000"
const checksDirectory   = "/home/mbodjiki/checks"


func main() {
    l, err := net.Listen("tcp", listenAddr)
    if err != nil {
        log.Fatal(err)
    }
    for {
        c, err := l.Accept()
        if err != nil {
            log.Fatal(err)
        }

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
