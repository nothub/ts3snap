//go:build e2e

package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	tsraw "github.com/multiplay/go-ts3"
)

func TestRoundtrip(t *testing.T) {
	eachProto(t, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.createChannel(t, "e2e")
		s.mustRun(t, proto, "create", file)
		s.deleteChannel(t, "e2e")
		s.mustRun(t, proto, "deploy", file)

		if !s.hasChannel(t, "e2e") {
			t.Fatal("channel missing after deploy")
		}
	})
}

func TestEncrypted(t *testing.T) {
	eachProto(t, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.createChannel(t, "e2e")
		s.mustRun(t, proto, "--secret=e2e-secret", "create", file)
		s.deleteChannel(t, "e2e")
		s.mustRun(t, proto, "--secret=e2e-secret", "deploy", file)

		if !s.hasChannel(t, "e2e") {
			t.Fatal("channel missing after deploy")
		}
	})
}

func TestWrongSecret(t *testing.T) {
	eachProto(t, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.createChannel(t, "e2e")
		s.mustRun(t, proto, "--secret=e2e-secret", "create", file)
		s.deleteChannel(t, "e2e")

		if _, err := s.ts3snap(proto, "--secret=wrong", "deploy", file); err == nil {
			t.Fatal("deploy with wrong secret succeeded")
		}
		if s.hasChannel(t, "e2e") {
			t.Fatal("channel restored by deploy with wrong secret")
		}
	})
}

// The secret is a query parameter, so characters with a meaning in the
// query syntax (space, pipe, backslash, equals) must survive the transport.
// Each direction is checked against the harness's own escaped query, since
// a roundtrip through ts3snap alone passes when both sides mangle alike.
const specialSecret = `e2e secret|with\special=chars/äö`

func TestSecretSpecialCharsCreate(t *testing.T) {
	eachProto(t, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.createChannel(t, "e2e")
		s.mustRun(t, proto, "--secret="+specialSecret, "create", file)
		s.deleteChannel(t, "e2e")
		s.deploySnapshot(t, file, specialSecret)

		if !s.hasChannel(t, "e2e") {
			t.Fatal("channel missing after deploy")
		}
	})
}

func TestSecretSpecialCharsDeploy(t *testing.T) {
	eachProto(t, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.createChannel(t, "e2e")
		s.createSnapshot(t, file, specialSecret)
		s.deleteChannel(t, "e2e")
		s.mustRun(t, proto, "--secret="+specialSecret, "deploy", file)

		if !s.hasChannel(t, "e2e") {
			t.Fatal("channel missing after deploy")
		}
	})
}

func TestFreshServer(t *testing.T) {
	eachProto(t, func(t *testing.T, src *server, proto string) {
		dst := newServer(t)
		file := filepath.Join(t.TempDir(), "snapshot.json")

		src.createChannel(t, "e2e")
		src.mustRun(t, proto, "create", file)
		dst.mustRun(t, proto, "deploy", file)

		if !dst.hasChannel(t, "e2e") {
			t.Fatal("channel missing on fresh server after deploy")
		}
	})
}

func TestFullContent(t *testing.T) {
	eachProto(t, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.mustQuery(t, tsraw.NewCmd("serveredit").WithArgs(
			tsraw.NewArg("virtualserver_name", "e2e name"),
			tsraw.NewArg("virtualserver_welcomemessage", "e2e welcome"),
		))
		sgid := s.createGroup(t, "e2e group")
		s.mustQuery(t, tsraw.NewCmd("servergroupaddperm").WithArgs(
			tsraw.NewArg("sgid", sgid),
			tsraw.NewArg("permsid", "b_client_info_view"),
			tsraw.NewArg("permvalue", 1),
			tsraw.NewArg("permnegated", 0),
			tsraw.NewArg("permskip", 0),
		))
		parent := s.createChannel(t, "e2e parent")
		s.mustQuery(t, tsraw.NewCmd("channelcreate").WithArgs(
			tsraw.NewArg("channel_name", "e2e child"),
			tsraw.NewArg("channel_topic", "e2e topic"),
			tsraw.NewArg("channel_flag_permanent", 1),
			tsraw.NewArg("cpid", parent),
		))

		want := s.state(t)
		s.mustRun(t, proto, "create", file)

		s.mustQuery(t, tsraw.NewCmd("serveredit").WithArgs(
			tsraw.NewArg("virtualserver_name", "changed"),
			tsraw.NewArg("virtualserver_welcomemessage", "changed"),
		))
		s.mustQuery(t, tsraw.NewCmd("servergroupdel").WithArgs(tsraw.NewArg("sgid", sgid), tsraw.NewArg("force", 1)))
		s.deleteChannel(t, "e2e parent")
		if slices.Equal(s.state(t), want) {
			t.Fatal("state unchanged before deploy, test proves nothing")
		}

		s.mustRun(t, proto, "deploy", file)

		if got := s.state(t); !slices.Equal(got, want) {
			t.Fatalf("state after deploy differs\ngot:  %q\nwant: %q", got, want)
		}
	})
}

func TestKeepFiles(t *testing.T) {
	eachProto(t, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		cid := s.createChannel(t, "e2e")
		s.mustRun(t, proto, "create", file)
		s.createDir(t, cid, "e2e-dir")
		s.mustRun(t, proto, "--keepfiles", "deploy", file)

		cid = s.channelID(t, "e2e")
		if !slices.Contains(s.files(t, cid), "e2e-dir") {
			t.Fatal("channel files lost despite --keepfiles")
		}
	})
}

// Control for TestKeepFiles: without the flag, deploy drops channel files.
func TestDeployDropsFiles(t *testing.T) {
	eachProto(t, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		cid := s.createChannel(t, "e2e")
		s.mustRun(t, proto, "create", file)
		s.createDir(t, cid, "e2e-dir")
		s.mustRun(t, proto, "deploy", file)

		cid = s.channelID(t, "e2e")
		if slices.Contains(s.files(t, cid), "e2e-dir") {
			t.Fatal("channel files kept without --keepfiles")
		}
	})
}

// snapshotFile is the file format ts3snap reads and writes.
type snapshotFile struct {
	Version int    `json:"version,string" ms:"version"`
	Salt    string `json:"salt" ms:"salt"`
	Data    string `json:"data" ms:"data"`
}

func (s *server) createSnapshot(t *testing.T, file, secret string) {
	t.Helper()
	var snap snapshotFile
	err := s.query(func(c *tsraw.Client) error {
		_, err := c.ExecCmd(tsraw.NewCmd("serversnapshotcreate").
			WithArgs(tsraw.NewArg("password", secret)).WithResponse(&snap))
		return err
	})
	if err != nil {
		t.Fatalf("serversnapshotcreate: %v", err)
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (s *server) deploySnapshot(t *testing.T, file, secret string) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var snap snapshotFile
	if err := json.Unmarshal(b, &snap); err != nil {
		t.Fatal(err)
	}
	s.mustQuery(t, tsraw.NewCmd("serversnapshotdeploy").WithOptions("-mapping").WithArgs(
		tsraw.NewArg("password", secret),
		tsraw.NewArg("version", snap.Version),
		tsraw.NewArg("salt", snap.Salt),
		tsraw.NewArg("data", snap.Data),
	))
}

type channel struct {
	ID       int    `ms:"cid"`
	ParentID int    `ms:"pid"`
	Name     string `ms:"channel_name"`
	Topic    string `ms:"channel_topic"`
}

func (s *server) channels(t *testing.T) []channel {
	t.Helper()
	var chans []channel
	err := s.read(func(c *tsraw.Client) error {
		chans = nil
		_, err := c.ExecCmd(tsraw.NewCmd("channellist").WithOptions("-topic").WithResponse(&chans))
		return err
	})
	if err != nil {
		t.Fatalf("channellist: %v", err)
	}
	return chans
}

func (s *server) createChannel(t *testing.T, name string) int {
	t.Helper()
	var resp struct {
		ID int `ms:"cid"`
	}
	err := s.query(func(c *tsraw.Client) error {
		_, err := c.ExecCmd(tsraw.NewCmd("channelcreate").WithArgs(
			tsraw.NewArg("channel_name", name),
			tsraw.NewArg("channel_flag_permanent", 1),
		).WithResponse(&resp))
		return err
	})
	if err != nil {
		t.Fatalf("create channel %q: %v", name, err)
	}
	return resp.ID
}

func (s *server) channelID(t *testing.T, name string) int {
	t.Helper()
	chans := s.channels(t)
	i := slices.IndexFunc(chans, func(ch channel) bool { return ch.Name == name })
	if i < 0 {
		t.Fatalf("no channel %q", name)
	}
	return chans[i].ID
}

func (s *server) hasChannel(t *testing.T, name string) bool {
	t.Helper()
	return slices.ContainsFunc(s.channels(t), func(ch channel) bool { return ch.Name == name })
}

func (s *server) deleteChannel(t *testing.T, name string) {
	t.Helper()
	s.mustQuery(t, tsraw.NewCmd("channeldelete").WithArgs(
		tsraw.NewArg("cid", s.channelID(t, name)),
		tsraw.NewArg("force", 1),
	))
}

func (s *server) createGroup(t *testing.T, name string) int {
	t.Helper()
	var resp struct {
		ID int `ms:"sgid"`
	}
	err := s.query(func(c *tsraw.Client) error {
		_, err := c.ExecCmd(tsraw.NewCmd("servergroupadd").WithArgs(tsraw.NewArg("name", name)).WithResponse(&resp))
		return err
	})
	if err != nil {
		t.Fatalf("create group %q: %v", name, err)
	}
	return resp.ID
}

func (s *server) createDir(t *testing.T, cid int, name string) {
	t.Helper()
	s.mustQuery(t, tsraw.NewCmd("ftcreatedir").WithArgs(
		tsraw.NewArg("cid", cid),
		tsraw.NewArg("cpw", ""),
		tsraw.NewArg("dirname", "/"+name),
	))
}

func (s *server) files(t *testing.T, cid int) []string {
	t.Helper()
	var entries []struct {
		Name string `ms:"name"`
	}
	err := s.read(func(c *tsraw.Client) error {
		entries = nil
		_, err := c.ExecCmd(tsraw.NewCmd("ftgetfilelist").WithArgs(
			tsraw.NewArg("cid", cid),
			tsraw.NewArg("cpw", ""),
			tsraw.NewArg("path", "/"),
		).WithResponse(&entries))
		// An empty channel answers with error 1281 (database empty result set).
		var qerr *tsraw.Error
		if errors.As(err, &qerr) && qerr.ID == 1281 {
			return nil
		}
		return err
	})
	if err != nil {
		t.Fatalf("ftgetfilelist: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return names
}

// state is a comparable fingerprint of what a snapshot should restore.
// Channels are keyed by name and parent name, since deploy with -mapping
// assigns new channel ids.
func (s *server) state(t *testing.T) []string {
	t.Helper()
	var info struct {
		Name    string `ms:"virtualserver_name"`
		Welcome string `ms:"virtualserver_welcomemessage"`
	}
	var groups []struct {
		ID   int    `ms:"sgid"`
		Name string `ms:"name"`
		Type int    `ms:"type"`
	}
	err := s.read(func(c *tsraw.Client) error {
		groups = nil
		if _, err := c.ExecCmd(tsraw.NewCmd("serverinfo").WithResponse(&info)); err != nil {
			return err
		}
		_, err := c.ExecCmd(tsraw.NewCmd("servergrouplist").WithResponse(&groups))
		return err
	})
	if err != nil {
		t.Fatalf("read server state: %v", err)
	}

	out := []string{"server name=" + info.Name, "server welcome=" + info.Welcome}

	for _, g := range groups {
		// type 1 is a regular group; templates (0) and query groups (2) are instance-wide.
		if g.Type != 1 {
			continue
		}
		out = append(out, "group "+g.Name)
		var perms []struct {
			ID    string `ms:"permsid"`
			Value int    `ms:"permvalue"`
		}
		err := s.read(func(c *tsraw.Client) error {
			perms = nil
			_, err := c.ExecCmd(tsraw.NewCmd("servergrouppermlist").
				WithArgs(tsraw.NewArg("sgid", g.ID)).WithOptions("-permsid").WithResponse(&perms))
			return err
		})
		if err != nil {
			t.Fatalf("servergrouppermlist %q: %v", g.Name, err)
		}
		for _, p := range perms {
			out = append(out, fmt.Sprintf("group %s perm %s=%d", g.Name, p.ID, p.Value))
		}
	}

	chans := s.channels(t)
	names := map[int]string{}
	for _, ch := range chans {
		names[ch.ID] = ch.Name
	}
	for _, ch := range chans {
		out = append(out, fmt.Sprintf("channel %s parent=%q topic=%q", ch.Name, names[ch.ParentID], ch.Topic))
	}

	slices.Sort(out)
	return out
}
