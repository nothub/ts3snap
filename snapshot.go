package main

import (
	"encoding/json"
	"log"
	"os"
)

type Snapshot struct {
	Version int    `json:"version,string"`
	Salt    string `json:"salt"`
	Data    string `json:"data"`
}

func save(snap Snapshot, filePath string) {
	log.Printf("Saving file: %s\n", filePath)

	bytes, err := json.Marshal(snap)
	if err != nil {
		log.Fatalln(err.Error())
	}

	err = os.WriteFile(filePath, bytes, 0640)
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func load(filePath string) (snap Snapshot) {
	log.Printf("Loading file: %s\n", filePath)

	bytes, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalln(err.Error())
	}

	err = json.Unmarshal(bytes, &snap)
	if err != nil {
		log.Fatalln(err.Error())
	}

	return snap
}
