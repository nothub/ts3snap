package main

import (
	tsweb "github.com/jkoenig134/go-ts3"
	"log"
)

func createWeb(client *tsweb.TeamspeakHttpClient) Snapshot {

	snap, err := client.ServerSnapshotCreate(tsweb.ServerSnapshotCreateRequest{Password: secret})
	if err != nil {
		log.Fatalln(err.Error())
	}

	return Snapshot{
		Version: snap.Version,
		Salt:    snap.Salt,
		Data:    snap.Data,
	}
}

func deployWeb(client *tsweb.TeamspeakHttpClient, snap Snapshot) {

	var request = tsweb.ServerSnapshotDeployRequest{
		Data:      snap.Data,
		Salt:      snap.Salt,
		Version:   snap.Version,
		Password:  secret,
		Mapping:   true,
		KeepFiles: keepFiles,
	}

	_, err := client.ServerSnapshotDeploy(request)
	if err != nil {
		log.Fatalln(err.Error())
	}

}
