package main

import (
	_ "embed"
	"flag"
	"fmt"
	tsweb "github.com/jkoenig134/go-ts3"
	tsraw "github.com/multiplay/go-ts3"
	"golang.org/x/crypto/ssh"
	"log"
	"net/url"
	"os"
	"path/filepath"
)

func main() {

	log.SetFlags(0)

	flags()

	cmd := flag.Arg(0)
	if cmd != "create" && cmd != "deploy" {
		log.Println("Invalid command!")
		flag.Usage()
		os.Exit(2)
	}

	filePath := flag.Arg(1)
	if filePath == "" {
		log.Println("Missing path argument!")
		flag.Usage()
		os.Exit(2)
	}
	{
		abs, err := filepath.Abs(filePath)
		if err != nil {
			log.Fatalln(err.Error())
		}
		filePath = abs
	}

	var createF func() Snapshot
	var deployF func(snap Snapshot)

	switch proto {
	case "raw", "ssh":

		if user == "" || pass == "" {
			log.Fatalln("Missing query user or password!")
		}

		addr := fmt.Sprintf("%s:%v", host, port)

		log.Printf("Connecting to: %s (%s)", addr, proto)

		var client *tsraw.Client
		{
			var err error
			switch proto {
			case "raw":
				client, err = tsraw.NewClient(addr)
			case "ssh":
				client, err = tsraw.NewClient(addr, tsraw.SSH(&ssh.ClientConfig{
					User: user,
					Auth: []ssh.AuthMethod{
						ssh.Password(pass),
					},
					HostKeyCallback:   ssh.InsecureIgnoreHostKey(),
					HostKeyAlgorithms: []string{"rsa-sha2-512"},
				}))
			}
			if err != nil {
				log.Fatalln(err.Error())
			}
		}
		defer client.Close()

		if proto == "raw" {
			err := client.Login(user, pass)
			if err != nil {
				log.Fatal(err)
			}
		}

		err := client.Use(int(srvId))
		if err != nil {
			log.Fatal(err)
		}

		createF = func() Snapshot { return createRaw(client) }
		deployF = func(snap Snapshot) { deployRaw(client, snap) }

	case "http", "https":

		if key == "" {
			log.Fatalln("Missing query key!")
		}

		j, err := url.JoinPath(fmt.Sprintf("%s://%s:%v", proto, host, port), urlPath)
		if err != nil {
			log.Fatalf("Invalid url path: %v\n", urlPath)
		}

		u, err := url.Parse(j)
		if err != nil {
			log.Fatalf("Invalid url: %v\n", urlPath)
		}

		log.Printf("Connecting to: %s", u.String())

		client := tsweb.NewClient(tsweb.NewConfig(u.String(), key))
		client.SetInsecure(sslInsecure)

		client.SetServerID(int(srvId))

		createF = func() Snapshot { return createWeb(&client) }
		deployF = func(data Snapshot) { deployWeb(&client, data) }

	default:
		log.Fatalf("Invalid server query protocol: %v", proto)
	}

	switch cmd {
	case "create":
		snap := createF()
		save(snap, filePath)
	case "deploy":
		snap := load(filePath)
		deployF(snap)
	}

	log.Println("Done!")
}
