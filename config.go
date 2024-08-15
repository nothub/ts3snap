package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
)

var host string
var port uint
var proto string
var user string
var pass string
var key string
var secret string
var srvId uint
var urlPath string
var sslInsecure bool

var keepFiles bool

//go:embed USAGE.txt
var usage string

func flags() {

	flag.Usage = func() {
		log.Print(usage)
	}

	// global options
	flag.StringVar(&host, "host", "localhost", "")
	flag.UintVar(&port, "port", 10022, "")
	flag.StringVar(&proto, "proto", "", "")
	flag.StringVar(&user, "user", "serveradmin", "")
	flag.StringVar(&pass, "pass", "", "")
	flag.StringVar(&key, "key", "", "")
	flag.StringVar(&secret, "secret", "", "")
	flag.UintVar(&srvId, "srvid", 1, "")
	flag.StringVar(&urlPath, "url-path", "", "")
	flag.BoolVar(&sslInsecure, "ssl-insecure", false, "")

	// command options (deploy)
	flag.BoolVar(&keepFiles, "keepfiles", false, "")

	// this is annoying
	h1 := flag.Bool("h", false, "")
	h2 := flag.Bool("help", false, "")

	flag.Parse()

	// help requested
	if *h1 || *h2 || flag.Arg(0) == "help" {
		fmt.Print(usage)
		os.Exit(0)
	}

	// guess protocol by port
	if proto == "" {
		switch port {
		case 10011:
			proto = "raw"
		case 10022:
			proto = "ssh"
		case 80, 8080, 10080:
			proto = "http"
		case 443, 8443, 10443:
			proto = "https"
		default:
			log.Println("Unable to guess protocol!")
			log.Fatalln("Protocol flag (--proto) is required.")
		}
	}

	// validate

	if host == "" {
		log.Fatalf("Invalid server query address: %v", host)
	}

	if port < 1 || port > math.MaxUint16 {
		log.Fatalf("Invalid server query port: %v", port)
	}

	if srvId < 1 {
		log.Fatalf("Invalid virtual server id: %v\n", srvId)
	}

}
