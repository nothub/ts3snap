//go:build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	tsraw "github.com/multiplay/go-ts3"
)

var protocols = []string{"raw", "ssh", "http", "https"}

func TestRoundtrip(t *testing.T) {
	for _, proto := range protocols {
		t.Run(proto, func(t *testing.T) {
			name := "e2e-roundtrip-" + proto
			file := filepath.Join(t.TempDir(), "snapshot.json")

			createChannel(t, name)
			mustRun(t, proto, "create", file)
			deleteChannel(t, name)
			mustRun(t, proto, "deploy", file)

			if !hasChannel(t, name) {
				t.Fatalf("channel %q missing after deploy", name)
			}
		})
	}
}

func TestEncrypted(t *testing.T) {
	for _, proto := range protocols {
		t.Run(proto, func(t *testing.T) {
			name := "e2e-encrypted-" + proto
			file := filepath.Join(t.TempDir(), "snapshot.json")

			createChannel(t, name)
			mustRun(t, proto, "--secret=e2e-secret", "create", file)
			deleteChannel(t, name)

			if err := ts3snap(proto, "--secret=wrong", "deploy", file); err == nil {
				t.Fatal("deploy with wrong secret succeeded")
			}
			if hasChannel(t, name) {
				t.Fatalf("channel %q restored by deploy with wrong secret", name)
			}

			mustRun(t, proto, "--secret=e2e-secret", "deploy", file)
			if !hasChannel(t, name) {
				t.Fatalf("channel %q missing after deploy", name)
			}
		})
	}
}

func mustRun(t *testing.T, proto string, args ...string) {
	t.Helper()
	if err := ts3snap(proto, args...); err != nil {
		t.Fatal(err)
	}
}

func createChannel(t *testing.T, name string) {
	t.Helper()
	err := withQuery(func(c *tsraw.Client) error {
		_, err := c.ExecCmd(tsraw.NewCmd("channelcreate").WithArgs(
			tsraw.NewArg("channel_name", name),
			tsraw.NewArg("channel_flag_permanent", 1),
		))
		return err
	})
	if err != nil {
		t.Fatalf("create channel %q: %v", name, err)
	}
}

func deleteChannel(t *testing.T, name string) {
	t.Helper()
	err := withQuery(func(c *tsraw.Client) error {
		chans, err := c.Server.ChannelList()
		if err != nil {
			return err
		}
		i := slices.IndexFunc(chans, func(ch *tsraw.Channel) bool { return ch.ChannelName == name })
		if i < 0 {
			return fmt.Errorf("no such channel")
		}
		_, err = c.ExecCmd(tsraw.NewCmd("channeldelete").WithArgs(
			tsraw.NewArg("cid", chans[i].ID),
			tsraw.NewArg("force", 1),
		))
		return err
	})
	if err != nil {
		t.Fatalf("delete channel %q: %v", name, err)
	}
}

func hasChannel(t *testing.T, name string) bool {
	t.Helper()
	var found bool
	err := withQuery(func(c *tsraw.Client) error {
		chans, err := c.Server.ChannelList()
		found = slices.ContainsFunc(chans, func(ch *tsraw.Channel) bool { return ch.ChannelName == name })
		return err
	})
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	return found
}
