package tlscheck

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestNormalizeHost(t *testing.T) {
	ok := map[string]string{
		"tokoku.id":                    "tokoku.id",
		"  TokoKu.ID  ":                "tokoku.id",
		"https://studiorana.co.id/":    "studiorana.co.id",
		"http://api.kopisenja.com/a?b": "api.kopisenja.com",
		"example.com.":                 "example.com",
	}
	for in, want := range ok {
		got, err := NormalizeHost(in)
		if err != nil || got != want {
			t.Errorf("NormalizeHost(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{"", "localhost", "db", "127.0.0.1", "[::1]", "10.0.0.1", "example.com:8443",
		"user@example.com", "-bad.com", "bad-.com", "a..b", "exa mple.com", "tökö.id"}
	for _, in := range bad {
		if _, err := NormalizeHost(in); !errors.Is(err, ErrInvalidHost) {
			t.Errorf("NormalizeHost(%q) err = %v, want ErrInvalidHost", in, err)
		}
	}
}

func TestPublic(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.1.2.3", "172.17.0.2", "192.168.1.10", "169.254.169.254",
		"100.64.0.1", "0.0.0.0", "198.18.0.1", "203.0.113.5", "224.0.0.1", "255.255.255.255",
		"::1", "::", "fe80::1", "fc00::1", "fd12:3456::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "2001:db8::1"}
	for _, s := range blocked {
		if Public(netip.MustParseAddr(s)) {
			t.Errorf("Public(%s) = true, want false", s)
		}
	}
	allowed := []string{"1.1.1.1", "93.184.215.14", "8.8.8.8", "2606:4700:4700::1111", "::ffff:1.1.1.1"}
	for _, s := range allowed {
		if !Public(netip.MustParseAddr(s)) {
			t.Errorf("Public(%s) = false, want true", s)
		}
	}
}

// stubChecker points every lookup at the given addresses and port.
func stubChecker(port string, roots *x509.CertPool, addrs ...string) *Checker {
	c := New()
	c.Lookup = func(context.Context, string) ([]netip.Addr, error) {
		out := make([]netip.Addr, len(addrs))
		for i, a := range addrs {
			out[i] = netip.MustParseAddr(a)
		}
		return out, nil
	}
	c.Port = port
	c.Roots = roots
	return c
}

func TestCheckRefusesLocalAnswers(t *testing.T) {
	cases := [][]string{{"127.0.0.1"}, {"10.0.0.5"}, {"1.1.1.1", "172.18.0.3"}, {"169.254.169.254"}}
	for _, addrs := range cases {
		c := stubChecker("443", nil, addrs...)
		if _, err := c.Check(context.Background(), "example.com"); !errors.Is(err, ErrBlockedHost) {
			t.Errorf("answers %v: err = %v, want ErrBlockedHost", addrs, err)
		}
	}
}

func TestCheckRefusesInvalidHostBeforeLookup(t *testing.T) {
	c := New()
	c.Lookup = func(context.Context, string) ([]netip.Addr, error) {
		t.Fatal("lookup must not run for an invalid host")
		return nil, nil
	}
	for _, h := range []string{"localhost", "127.0.0.1", "db"} {
		if _, err := c.Check(context.Background(), h); !errors.Is(err, ErrInvalidHost) {
			t.Errorf("Check(%q) err = %v, want ErrInvalidHost", h, err)
		}
	}
}

func TestCheckReadsValidCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	c := stubChecker(port, roots, "127.0.0.1")
	c.Allowed = func(netip.Addr) bool { return true } // the test server is on loopback

	// The httptest certificate is issued for example.com.
	res, err := c.Check(context.Background(), "https://example.com/")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.ChainValid {
		t.Errorf("ChainValid = false (%s), want true", res.ChainError)
	}
	if !res.NotAfter.Equal(srv.Certificate().NotAfter) {
		t.Errorf("NotAfter = %v, want %v", res.NotAfter, srv.Certificate().NotAfter)
	}
	if res.Host != "example.com" || res.Address != "127.0.0.1" {
		t.Errorf("host/address = %s/%s", res.Host, res.Address)
	}
}

func TestCheckReadsExpiredCertificate(t *testing.T) {
	notAfter := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	cert, leaf := selfSigned(t, "expired.test", notAfter)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = conn.(*tls.Conn).Handshake()
				_ = conn.Close()
			}()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	c := stubChecker(port, roots, "127.0.0.1")
	c.Allowed = func(netip.Addr) bool { return true }

	res, err := c.Check(context.Background(), "expired.test")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.ChainValid || res.ChainError == "" {
		t.Errorf("expired certificate reported as valid")
	}
	if !res.NotAfter.Equal(notAfter) {
		t.Errorf("NotAfter = %v, want %v", res.NotAfter, notAfter)
	}
	if res.Issuer != "Tenggat Test · expired.test" {
		t.Errorf("Issuer = %q", res.Issuer)
	}
}

func TestCheckUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close() // nothing listens on this port any more

	c := stubChecker(port, nil, "127.0.0.1")
	c.Allowed = func(netip.Addr) bool { return true }
	if _, err := c.Check(context.Background(), "example.com"); !errors.Is(err, ErrUnreachable) {
		t.Errorf("err = %v, want ErrUnreachable", err)
	}
}

func selfSigned(t *testing.T, host string, notAfter time.Time) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: host, Organization: []string{"Tenggat Test"}},
		Issuer:                pkix.Name{CommonName: host, Organization: []string{"Tenggat Test"}},
		DNSNames:              []string{host},
		NotBefore:             notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, leaf
}

// TestCheckLiveHost talks to a real server; it runs only with TENGGAT_NETWORK_TEST=1.
func TestCheckLiveHost(t *testing.T) {
	if os.Getenv("TENGGAT_NETWORK_TEST") != "1" {
		t.Skip("set TENGGAT_NETWORK_TEST=1 to run against the internet")
	}
	res, err := New().Check(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.ChainValid || res.NotAfter.Before(time.Now()) {
		t.Errorf("unexpected result: %+v", res)
	}
	t.Logf("example.com: %s, ends %s, via %s", res.Issuer, res.NotAfter.Format(time.DateOnly), res.Address)
}

func TestIssuerName(t *testing.T) {
	for _, c := range []struct{ org, cn, want string }{
		{"Sectigo Limited", "Sectigo Public Server Authentication CA DV E36", "Sectigo Public Server Authentication CA DV E36"},
		{"Amazon", "Amazon RSA 2048 M04", "Amazon RSA 2048 M04"},
		{"Let's Encrypt", "R12", "Let's Encrypt · R12"},
		{"Google Trust Services", "WE1", "Google Trust Services · WE1"},
		{"", "Root CA", "Root CA"},
		{"DigiCert Inc", "", "DigiCert Inc"},
		{"Same", "Same", "Same"},
	} {
		cert := &x509.Certificate{Issuer: pkix.Name{CommonName: c.cn}}
		if c.org != "" {
			cert.Issuer.Organization = []string{c.org}
		}
		if got := issuerName(cert); got != c.want {
			t.Errorf("issuerName(%q, %q) = %q, want %q", c.org, c.cn, got, c.want)
		}
	}
}
