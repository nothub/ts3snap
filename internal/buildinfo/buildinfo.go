package buildinfo

import (
	"fmt"
	"log"
	"path"
	"runtime/debug"
	"strings"
)

const unknown string = "unknown"

var (
	name   = unknown
	module = unknown

	version = unknown
	commit  = unknown
	date    = unknown

	arch     = unknown
	os       = unknown
	compiler = unknown
)

func init() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		log.Fatalln("unable to read build info from binary")
	}

	name = path.Base(bi.Main.Path)
	module = bi.Main.Path

	var dirty = false

	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.modified":
			if kv.Value == "true" {
				dirty = true
			}
		case "GOARCH":
			arch = kv.Value
		case "GOOS":
			os = kv.Value
		case "-compiler":
			compiler = kv.Value
		}
	}

	if dirty {
		version = version + "+DIRTY"
	}
}

func Name() string {
	return name
}

func Module() string {
	return module
}

func String() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("%s\n", version))
	sb.WriteString(fmt.Sprintf("built from: %s\n", commit))
	sb.WriteString(fmt.Sprintf("built for:  %s-%s-%s\n", arch, os, compiler))
	sb.WriteString(fmt.Sprintf("built at:   %s\n", date))
	return sb.String()
}
