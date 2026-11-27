package main

import (
	"encoding/json"
	"log"
	"os"
	"path"
)

type Snapshot struct {
	Version int    `json:"version,string"`
	Salt    string `json:"salt"`
	Data    string `json:"data"`
	// Files holds server files that TeamSpeak leaves out of its snapshot,
	// keyed by path in channel 0. With a secret they go to FilesSealed instead.
	Files       map[string][]byte `json:"files,omitempty"`
	FilesSealed []byte            `json:"files_sealed,omitempty"`
}

func (snap *Snapshot) setFiles(files map[string][]byte, secret string) {
	if len(files) == 0 {
		return
	}
	if secret == "" {
		snap.Files = files
		return
	}
	plain, err := json.Marshal(files)
	if err != nil {
		log.Fatalln(err.Error())
	}
	snap.FilesSealed, err = seal(secret, plain)
	if err != nil {
		log.Fatalln(err.Error())
	}
}

func (snap *Snapshot) files(secret string) map[string][]byte {
	files := snap.Files
	if snap.FilesSealed != nil {
		if secret == "" {
			log.Fatalln("Snapshot contains encrypted files, secret (--secret) is required!")
		}
		plain, err := unseal(secret, snap.FilesSealed)
		if err != nil {
			log.Fatalln("Unable to decrypt snapshot files, wrong secret?")
		}
		err = json.Unmarshal(plain, &files)
		if err != nil {
			log.Fatalln(err.Error())
		}
	}
	for p := range files {
		if path.Dir(p) != iconDir {
			log.Fatalf("Unsupported file in snapshot: %s", p)
		}
	}
	return files
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
