//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tsraw "github.com/multiplay/go-ts3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	image     = "n0thub/ts3:3.13.8"
	adminUser = "serveradmin"
	adminPass = "e2e-pass"
	crtPath   = "/etc/ssl/query/crt.pem"
	keyPath   = "/etc/ssl/query/key.pem"
	ftPort    = "30033/tcp"
)

var protocols = []string{"raw", "ssh", "http", "https"}

var queryPorts = map[string]string{
	"raw":   "10011/tcp",
	"ssh":   "10022/tcp",
	"http":  "10080/tcp",
	"https": "10443/tcp",
}

// Set once by TestMain, read-only afterwards.
var (
	bin      string
	crt, key []byte
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "ts3snap-e2e-")
	if err != nil {
		log.Print(err)
		return 1
	}
	defer os.RemoveAll(dir)

	bin = filepath.Join(dir, "ts3snap")
	// e2e is its own module, so the binary is built from inside the root module.
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("build ts3snap: %v\n%s", err, out)
		return 1
	}

	crt, key, err = selfSigned()
	if err != nil {
		log.Print(err)
		return 1
	}

	return m.Run()
}

// server is one TeamSpeak container, owned by a single test.
type server struct {
	host   string
	ports  map[string]string
	ftPort string
	apiKey string
}

// eachProto runs fn once per query protocol, each in parallel against its own server.
func eachProto(t *testing.T, fn func(t *testing.T, s *server, proto string)) {
	eachProtoIn(t, protocols, fn)
}

// eachProtoIn is eachProto limited to protos.
func eachProtoIn(t *testing.T, protos []string, fn func(t *testing.T, s *server, proto string)) {
	t.Parallel()
	for _, proto := range protos {
		t.Run(proto, func(t *testing.T) {
			t.Parallel()
			fn(t, newServer(t), proto)
		})
	}
}

func newServer(t *testing.T, opts ...testcontainers.ContainerCustomizer) *server {
	t.Helper()

	var waits []wait.Strategy
	exposed := []string{ftPort}
	for _, p := range queryPorts {
		exposed = append(exposed, p)
		waits = append(waits, wait.ForListeningPort(p))
	}

	ctr, err := testcontainers.Run(t.Context(), image, append([]testcontainers.ContainerCustomizer{
		testcontainers.WithEnv(map[string]string{"TS3SERVER_LICENSE": "accept"}),
		testcontainers.WithCmd(
			"serveradmin_password="+adminPass,
			"query_protocols=raw,ssh,http,https",
			"query_https_certificate_file="+crtPath,
			"query_https_private_key_file="+keyPath,
		),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{Reader: bytes.NewReader(crt), ContainerFilePath: crtPath, FileMode: 0o644},
			testcontainers.ContainerFile{Reader: bytes.NewReader(key), ContainerFilePath: keyPath, FileMode: 0o644},
		),
		testcontainers.WithExposedPorts(exposed...),
		testcontainers.WithWaitStrategyAndDeadline(2*time.Minute, waits...),
	}, opts...)...)
	testcontainers.CleanupContainer(t, ctr)
	// Registered after CleanupContainer, so it runs before the container is gone.
	t.Cleanup(func() {
		if !t.Failed() || ctr == nil {
			return
		}
		r, err := ctr.Logs(context.Background())
		if err != nil {
			t.Logf("container logs: %v", err)
			return
		}
		defer r.Close()
		b, _ := io.ReadAll(r)
		t.Logf("container logs:\n%s", b)
	})
	if err != nil {
		t.Fatalf("start %s: %v", image, err)
	}

	s := &server{ports: map[string]string{}}
	s.host, err = ctr.Host(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for proto, p := range queryPorts {
		mapped, err := ctr.MappedPort(t.Context(), p)
		if err != nil {
			t.Fatal(err)
		}
		s.ports[proto] = mapped.Port()
	}
	mapped, err := ctr.MappedPort(t.Context(), ftPort)
	if err != nil {
		t.Fatal(err)
	}
	s.ftPort = mapped.Port()

	s.apiKey, err = s.createAPIKey()
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return s
}

func (s *server) createAPIKey() (string, error) {
	var resp struct {
		Key string `ms:"apikey"`
	}
	err := s.query(func(c *tsraw.Client) error {
		_, err := c.ExecCmd(tsraw.NewCmd("apikeyadd").
			WithArgs(tsraw.NewArg("scope", "manage"), tsraw.NewArg("lifetime", 1)).
			WithResponse(&resp))
		return err
	})
	if err == nil && resp.Key == "" {
		err = errors.New("empty apikeyadd response")
	}
	return resp.Key, err
}

// query runs fn on a fresh raw query session bound to virtual server 1.
// A fresh session per call keeps a deploy from invalidating a long-lived one.
func (s *server) query(fn func(c *tsraw.Client) error) error {
	// Right after a deploy the server sometimes closes a new connection before its
	// banner (client: header: unexpected EOF), so the connect alone is retried.
	var c *tsraw.Client
	var err error
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(250 * time.Millisecond) {
		c, err = tsraw.NewClient(s.host + ":" + s.ports["raw"])
		if err == nil || time.Now().After(deadline) {
			break
		}
	}
	if err != nil {
		return err
	}
	defer c.Close()
	if err := retryFlood(func() error { return c.Login(adminUser, adminPass) }); err != nil {
		return err
	}
	if err := retryFlood(func() error { return c.Use(1) }); err != nil {
		return err
	}
	return retryFlood(func() error { return fn(c) })
}

// floodWindow is the server's default serverinstance_serverquery_flood_time.
const floodWindow = 3 * time.Second

// retryFlood runs fn until the server stops rejecting it for flooding (524).
// A rejected command was not run, so a retry cannot apply a write twice.
// Quicker retries than one flood window get the IP banned.
func retryFlood(fn func() error) error {
	for i := 0; ; i++ {
		err := fn()
		var qerr *tsraw.Error
		if !errors.As(err, &qerr) || qerr.ID != 524 || i == 10 {
			return err
		}
		time.Sleep(floodWindow)
	}
}

// read is query for a read-only fn. A deploy restarts the virtual server,
// which kicks sessions mid-command, so the whole session is retried.
// Writes go through query alone, since a retry could apply them twice.
// fn must reset whatever it decodes into, as it may run more than once.
func (s *server) read(fn func(c *tsraw.Client) error) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := s.query(fn)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// mustQuery runs a single query command and fails the test on error.
func (s *server) mustQuery(t *testing.T, cmd *tsraw.Cmd) {
	t.Helper()
	err := s.query(func(c *tsraw.Client) error {
		_, err := c.ExecCmd(cmd)
		return err
	})
	if err != nil {
		// The full command can carry a multi-kilobyte snapshot blob.
		name, _, _ := strings.Cut(cmd.String(), " ")
		t.Fatalf("%s: %v", name, err)
	}
}

// ts3snap runs the built binary against s over proto and returns its output.
func (s *server) ts3snap(proto string, args ...string) (string, error) {
	flags := []string{"--host=" + s.host, "--port=" + s.ports[proto], "--proto=" + proto, "--ft-port=" + s.ftPort}
	switch proto {
	case "raw", "ssh":
		flags = append(flags, "--user="+adminUser, "--pass="+adminPass)
	case "http", "https":
		flags = append(flags, "--key="+s.apiKey, "--ssl-insecure")
	}
	out, err := exec.Command(bin, append(flags, args...)...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("ts3snap %v: %w\n%s", args, err, out)
	}
	return string(out), nil
}

func (s *server) mustRun(t *testing.T, proto string, args ...string) string {
	t.Helper()
	out, err := s.ts3snap(proto, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func selfSigned() (crt, key []byte, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	crt = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	key = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	return crt, key, nil
}
