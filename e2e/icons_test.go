//go:build e2e

package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tsraw "github.com/multiplay/go-ts3"
	"github.com/testcontainers/testcontainers-go"
)

// WebQuery does not support file transfer, so icons only travel over these.
var ftProtocols = []string{"raw", "ssh"}

var webProtocols = []string{"http", "https"}

// Icon content is opaque to the server; binary bytes catch encoding slips.
var wantIcons = map[string][]byte{
	"icon_42":   {0x89, 'P', 'N', 'G', 0x00, 0x0a, 0xff},
	"icon_1337": []byte("second icon"),
}

func TestIcons(t *testing.T) {
	eachProtoIn(t, ftProtocols, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.uploadIcons(t, wantIcons)
		s.mustRun(t, proto, "create", file)
		s.mustRun(t, proto, "deploy", file)

		s.assertIcons(t, wantIcons)
	})
}

func TestIconsFreshServer(t *testing.T) {
	eachProtoIn(t, ftProtocols, func(t *testing.T, src *server, proto string) {
		dst := newServer(t)
		file := filepath.Join(t.TempDir(), "snapshot.json")

		src.uploadIcons(t, wantIcons)
		src.mustRun(t, proto, "create", file)
		dst.mustRun(t, proto, "deploy", file)

		dst.assertIcons(t, wantIcons)
	})
}

func TestIconsEncrypted(t *testing.T) {
	eachProtoIn(t, ftProtocols, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.uploadIcons(t, wantIcons)
		s.mustRun(t, proto, "--secret=e2e-secret", "create", file)

		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["files"]; ok {
			t.Fatal("encrypted snapshot has plaintext files field")
		}
		if _, ok := fields["files_sealed"]; !ok {
			t.Fatal("encrypted snapshot has no files_sealed field")
		}

		s.mustRun(t, proto, "--secret=e2e-secret", "deploy", file)

		s.assertIcons(t, wantIcons)
	})
}

func TestIconsWrongSecret(t *testing.T) {
	eachProtoIn(t, ftProtocols, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.uploadIcons(t, wantIcons)
		s.mustRun(t, proto, "--secret=e2e-secret", "create", file)

		if _, err := s.ts3snap(proto, "--secret=wrong", "deploy", file); err == nil {
			t.Fatal("deploy with wrong secret succeeded")
		}
		// Deploy deletes all icons, so a rejected secret must stop before it.
		s.assertIcons(t, wantIcons)
	})
}

func TestIconsMissingSecret(t *testing.T) {
	eachProtoIn(t, ftProtocols, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.uploadIcons(t, wantIcons)
		s.mustRun(t, proto, "--secret=e2e-secret", "create", file)

		out, err := s.ts3snap(proto, "deploy", file)
		if err == nil {
			t.Fatal("deploy without secret succeeded")
		}
		// The server would reject the deploy too, so the message shows that
		// ts3snap stopped first.
		if !strings.Contains(out, "secret (--secret) is required") {
			t.Fatalf("no missing secret error in output:\n%s", out)
		}
		s.assertIcons(t, wantIcons)
	})
}

// TestIconsDamaged covers a snapshot whose server data is intact but whose
// icons do not decrypt. The deploy must not run, as it would delete the icons.
func TestIconsDamaged(t *testing.T) {
	eachProtoIn(t, ftProtocols, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")

		s.uploadIcons(t, wantIcons)
		s.mustRun(t, proto, "--secret=e2e-secret", "create", file)

		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		sealed, err := base64.StdEncoding.DecodeString(fields["files_sealed"].(string))
		if err != nil {
			t.Fatal(err)
		}
		sealed[len(sealed)-1] ^= 0xff
		fields["files_sealed"] = sealed
		raw, err = json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, raw, 0o600); err != nil {
			t.Fatal(err)
		}

		out, err := s.ts3snap(proto, "--secret=e2e-secret", "deploy", file)
		if err == nil {
			t.Fatal("deploy with damaged icons succeeded")
		}
		if !strings.Contains(out, "Unable to decrypt snapshot files") {
			t.Fatalf("no decrypt error in output:\n%s", out)
		}
		s.assertIcons(t, wantIcons)
	})
}

// TestIconsLarge uses an icon of the server's maximum icon size (larger ones
// fail with 2565 invalid size), which spans several TCP segments.
func TestIconsLarge(t *testing.T) {
	eachProtoIn(t, ftProtocols, func(t *testing.T, s *server, proto string) {
		file := filepath.Join(t.TempDir(), "snapshot.json")
		large := make([]byte, 8192)
		rand.Read(large)
		want := map[string][]byte{"icon_4242": large}

		s.uploadIcons(t, want)
		s.mustRun(t, proto, "create", file)
		s.mustRun(t, proto, "deploy", file)

		s.assertIcons(t, want)
	})
}

// TestIconsMany has enough icons that the per-icon commands of ts3snap run
// into the server's query flood limit.
func TestIconsMany(t *testing.T) {
	t.Parallel()
	for _, proto := range ftProtocols {
		t.Run(proto, func(t *testing.T) {
			t.Parallel()
			// The image exempts the docker gateway from flood protection. Moving
			// the exemption to localhost puts the tests under it.
			s := newServer(t, testcontainers.WithEnv(map[string]string{"QUERY_CLIENT": "127.0.0.1"}))
			file := filepath.Join(t.TempDir(), "snapshot.json")
			want := map[string][]byte{}
			for i := range 20 {
				want[fmt.Sprintf("icon_%d", 1000+i)] = []byte(fmt.Sprintf("icon %d", i))
			}

			// Right after a flood the server refuses new connections for a
			// moment, so each step waits out the window the step before used up.
			s.uploadIcons(t, want)
			time.Sleep(floodWindow)
			s.mustRun(t, proto, "create", file)
			time.Sleep(floodWindow)
			s.mustRun(t, proto, "deploy", file)
			time.Sleep(floodWindow)

			s.assertIcons(t, want)
		})
	}
}

// ftInit runs an ftinit* command on c. These report failure in the response's
// status field, not as a query error.
func ftInit(c *tsraw.Client, cmd string, args ...tsraw.CmdArg) (key string, size int, err error) {
	var resp struct {
		Key    string `ms:"ftkey"`
		Size   int    `ms:"size"`
		Status int    `ms:"status"`
		Msg    string `ms:"msg"`
	}
	// Every icon costs one command, so many icons run into the flood limit.
	err = retryFlood(func() error {
		_, err := c.ExecCmd(tsraw.NewCmd(cmd).WithArgs(args...).WithResponse(&resp))
		return err
	})
	if err == nil && resp.Status != 0 {
		err = fmt.Errorf("%s: %s (%d)", cmd, resp.Msg, resp.Status)
	}
	return resp.Key, resp.Size, err
}

func (s *server) ftConn(key string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", s.host+":"+s.ftPort, 5*time.Second)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(conn, key); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (s *server) uploadIcons(t *testing.T, icons map[string][]byte) {
	t.Helper()
	// One session for all icons keeps logins from piling onto the flood limit.
	err := s.query(func(c *tsraw.Client) error {
		for name, data := range icons {
			if err := s.uploadIcon(c, name, data); err != nil {
				return fmt.Errorf("upload %s: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (s *server) uploadIcon(c *tsraw.Client, name string, data []byte) error {
	key, _, err := ftInit(c, "ftinitupload",
		tsraw.NewArg("clientftfid", 1), tsraw.NewArg("name", "/"+name),
		tsraw.NewArg("cid", 0), tsraw.NewArg("cpw", ""), tsraw.NewArg("size", len(data)),
		tsraw.NewArg("overwrite", 1), tsraw.NewArg("resume", 0))
	if err != nil {
		return err
	}
	conn, err := s.ftConn(key)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write(data); err != nil {
		return err
	}
	// The server closes the connection once it has stored all bytes.
	_, err = io.Copy(io.Discard, conn)
	return err
}

// icons downloads every icon of virtual server 1.
func (s *server) icons(t *testing.T) map[string][]byte {
	t.Helper()
	var icons map[string][]byte
	err := s.read(func(c *tsraw.Client) error {
		icons = map[string][]byte{}
		var entries []struct {
			Name string `ms:"name"`
		}
		_, err := c.ExecCmd(tsraw.NewCmd("ftgetfilelist").WithArgs(
			tsraw.NewArg("cid", 0), tsraw.NewArg("cpw", ""), tsraw.NewArg("path", "/icons"),
		).WithResponse(&entries))
		var qerr *tsraw.Error
		if errors.As(err, &qerr) && qerr.ID == 1281 {
			return nil
		}
		if err != nil {
			return fmt.Errorf("list icons: %w", err)
		}
		for _, e := range entries {
			data, err := s.downloadIcon(c, e.Name)
			if err != nil {
				return fmt.Errorf("download %s: %w", e.Name, err)
			}
			icons[e.Name] = data
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return icons
}

func (s *server) downloadIcon(c *tsraw.Client, name string) ([]byte, error) {
	key, size, err := ftInit(c, "ftinitdownload",
		tsraw.NewArg("clientftfid", 2), tsraw.NewArg("name", "/"+name),
		tsraw.NewArg("cid", 0), tsraw.NewArg("cpw", ""), tsraw.NewArg("seekpos", 0))
	if err != nil {
		return nil, err
	}
	conn, err := s.ftConn(key)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	data := make([]byte, size)
	_, err = io.ReadFull(conn, data)
	return data, err
}

// assertIcons fails t unless the server holds exactly want.
func (s *server) assertIcons(t *testing.T, want map[string][]byte) {
	t.Helper()
	got := s.icons(t)
	if maps.EqualFunc(got, want, bytes.Equal) {
		return
	}
	// Icons can be large, so only names and sizes are printed.
	sizes := func(m map[string][]byte) map[string]int {
		out := map[string]int{}
		for k, v := range m {
			out[k] = len(v)
		}
		return out
	}
	t.Fatalf("icons differ (name: size)\ngot:  %v\nwant: %v", sizes(got), sizes(want))
}
