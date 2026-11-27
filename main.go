package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	tsweb "github.com/jkoenig134/go-ts3"
	tsraw "github.com/multiplay/go-ts3"
	"golang.org/x/crypto/ssh"
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
	// Left nil for WebQuery, which does not support file transfer.
	var backupIconsF func() map[string][]byte
	var restoreIconsF func(icons map[string][]byte)

	switch proto {
	case "raw", "ssh":

		if user == "" || pass == "" {
			log.Fatalln("Missing query user or password!")
		}

		addr := fmt.Sprintf("%s:%v", host, port)

		log.Printf("Connecting to: %s (%s)", addr, proto)

		connect := func() (*tsraw.Client, error) {
			var client *tsraw.Client
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
				return nil, err
			}

			if proto == "raw" {
				err := client.Login(user, pass)
				if err != nil {
					client.Close()
					return nil, err
				}
			}

			err = client.Use(int(srvId))
			if err != nil {
				client.Close()
				return nil, err
			}
			return client, nil
		}

		client, err := connect()
		if err != nil {
			log.Fatalln(err.Error())
		}
		defer func() { client.Close() }()

		ftAddr := fmt.Sprintf("%s:%v", host, ftPort)

		createF = func() Snapshot { return createRaw(client) }
		deployF = func(snap Snapshot) { deployRaw(client, snap) }
		backupIconsF = func() map[string][]byte {
			icons, err := backupIcons(client, ftAddr)
			if err != nil {
				log.Fatalln(err.Error())
			}
			return icons
		}
		restoreIconsF = func(icons map[string][]byte) {
			// The deploy restarts the virtual server, which drops this session.
			client.Close()
			var err error
			for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(time.Second) {
				client, err = connect()
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					log.Fatalln(err.Error())
				}
			}
			err = restoreIcons(client, ftAddr, icons)
			if err != nil {
				log.Fatalln(err.Error())
			}
		}

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
		if backupIconsF == nil {
			log.Println("Warning: icons are not included, WebQuery does not support file transfer. Use the raw or ssh protocol to include them.")
		} else {
			snap.setFiles(backupIconsF(), secret)
		}
		save(snap, filePath)
	case "deploy":
		snap := load(filePath)
		// Read first: a deploy deletes the server's icons, so unreadable
		// files have to fail before it.
		files := snap.files(secret)
		deployF(snap)
		if len(files) > 0 {
			if restoreIconsF == nil {
				log.Println("Warning: icons were not restored, WebQuery does not support file transfer. Use the raw or ssh protocol to restore them.")
			} else {
				restoreIconsF(files)
			}
		}
	}

	log.Println("Done!")
}
