package main

import (
	tsraw "github.com/multiplay/go-ts3"
	"log"
	"strconv"
	"strings"
)

func createRaw(client *tsraw.Client) Snapshot {

	cmd := strings.Builder{}
	cmd.WriteString("serversnapshotcreate")
	if secret != "" {
		cmd.WriteString(" password=" + secret)
	}

	var snap Snapshot
	_, err := client.ExecCmd(tsraw.NewCmd(cmd.String()).WithResponse(&snap))
	if err != nil {
		log.Fatalln(err.Error())
	}

	return snap
}

func deployRaw(client *tsraw.Client, snap Snapshot) {

	cmd := strings.Builder{}
	cmd.WriteString("serversnapshotdeploy")
	cmd.WriteString(" -mapping")
	if keepFiles {
		cmd.WriteString(" -keepfiles")
	}
	if secret != "" {
		cmd.WriteString(" password=" + secret)
	}
	cmd.WriteString(" version=" + strconv.Itoa(snap.Version))
	if snap.Salt != "" {
		cmd.WriteString(" salt=" + snap.Salt)
	}
	cmd.WriteString(" data=" + snap.Data)

	_, err := client.ExecCmd(tsraw.NewCmd(cmd.String()))
	if err != nil {
		log.Fatalln(err.Error())
	}

}
