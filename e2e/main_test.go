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
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
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
)

var queryPorts = map[string]string{
	"raw":   "10011/tcp",
	"ssh":   "10022/tcp",
	"http":  "10080/tcp",
	"https": "10443/tcp",
}

// Set once by TestMain, read-only afterwards.
var (
	bin    string
	host   string
	ports  = map[string]string{}
	apiKey string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	dir, err := os.MkdirTemp("", "ts3snap-e2e-")
	if err != nil {
		log.Print(err)
		return 1
	}
	defer os.RemoveAll(dir)

	bin = filepath.Join(dir, "ts3snap")
	out, err := exec.Command("go", "build", "-o", bin, "..").CombinedOutput()
	if err != nil {
		log.Printf("build ts3snap: %v\n%s", err, out)
		return 1
	}

	crt, key, err := selfSigned()
	if err != nil {
		log.Print(err)
		return 1
	}

	var waits []wait.Strategy
	var exposed []string
	for _, p := range queryPorts {
		exposed = append(exposed, p)
		waits = append(waits, wait.ForListeningPort(p))
	}

	ctr, err := testcontainers.Run(ctx, image,
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
	)
	defer func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			log.Print(err)
		}
	}()
	if err != nil {
		log.Printf("start %s: %v", image, err)
		return 1
	}

	host, err = ctr.Host(ctx)
	if err != nil {
		log.Print(err)
		return 1
	}
	for proto, p := range queryPorts {
		mapped, err := ctr.MappedPort(ctx, p)
		if err != nil {
			log.Print(err)
			return 1
		}
		ports[proto] = mapped.Port()
	}

	apiKey, err = createAPIKey()
	if err != nil {
		log.Printf("create api key: %v", err)
		return 1
	}

	return m.Run()
}

func createAPIKey() (string, error) {
	var resp struct {
		Key string `ms:"apikey"`
	}
	err := withQuery(func(c *tsraw.Client) error {
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

// withQuery runs fn on a fresh raw query session bound to virtual server 1.
// A fresh session per call keeps a deploy from invalidating a long-lived one.
func withQuery(fn func(c *tsraw.Client) error) error {
	c, err := tsraw.NewClient(host + ":" + ports["raw"])
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Login(adminUser, adminPass); err != nil {
		return err
	}
	if err := c.Use(1); err != nil {
		return err
	}
	return fn(c)
}

// ts3snap runs the built binary against the container over proto.
func ts3snap(proto string, args ...string) error {
	flags := []string{"--host=" + host, "--port=" + ports[proto], "--proto=" + proto}
	switch proto {
	case "raw", "ssh":
		flags = append(flags, "--user="+adminUser, "--pass="+adminPass)
	case "http", "https":
		flags = append(flags, "--key="+apiKey, "--ssl-insecure")
	}
	out, err := exec.Command(bin, append(flags, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ts3snap %v: %w\n%s", args, err, out)
	}
	return nil
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
