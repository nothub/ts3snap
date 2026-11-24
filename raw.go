package main

import (
	"log"

	tsraw "github.com/multiplay/go-ts3"
)

func createRaw(client *tsraw.Client) Snapshot {

	var args []tsraw.CmdArg
	if secret != "" {
		args = append(args, tsraw.NewArg("password", secret))
	}

	var snap Snapshot
	_, err := client.ExecCmd(tsraw.NewCmd("serversnapshotcreate").WithArgs(args...).WithResponse(&snap))
	if err != nil {
		log.Fatalln(err.Error())
	}

	return snap
}

func deployRaw(client *tsraw.Client, snap Snapshot) {

	opts := []string{"-mapping"}
	if keepFiles {
		opts = append(opts, "-keepfiles")
	}

	var args []tsraw.CmdArg
	if secret != "" {
		args = append(args, tsraw.NewArg("password", secret))
	}
	args = append(args, tsraw.NewArg("version", snap.Version))
	if snap.Salt != "" {
		args = append(args, tsraw.NewArg("salt", snap.Salt))
	}
	args = append(args, tsraw.NewArg("data", snap.Data))

	_, err := client.ExecCmd(tsraw.NewCmd("serversnapshotdeploy").WithArgs(args...).WithOptions(opts...))
	if err != nil {
		log.Fatalln(err.Error())
	}

}
