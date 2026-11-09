package rdap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

const verisignLike = `{
  "objectClassName": "domain",
  "ldhName": "GITHUB.COM",
  "events": [
    {"eventAction": "registration", "eventDate": "2007-10-09T18:20:50Z"},
    {"eventAction": "expiration", "eventDate": "2026-10-09T18:20:50Z"},
    {"eventAction": "last update of RDAP database", "eventDate": "2026-11-09T18:00:00Z"}
  ],
  "entities": [
    {"objectClassName": "entity", "handle": "292", "roles": ["registrar"],
     "vcardArray": ["vcard", [["version", {}, "text", "4.0"], ["fn", {}, "text", "MarkMonitor Inc."]]],
     "entities": [{"roles": ["abuse"], "vcardArray": ["vcard", [["fn", {}, "text", "Abuse desk"]]]}]}
  ]
}`

// testClient returns a client whose TLD servers all point at srv, with every
// name resolving to the test server's loopback address.
func testClient(srv *httptest.Server, tlds ...string) *Client {
	c := New()
	c.Servers = map[string]string{}
	for _, t := range tlds {
		c.Servers[t] = "https://example.com:" + srv.URL[strings.LastIndexByte(srv.URL, ':')+1:] + "/rdap/"
	}
	c.Resolve = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	c.Allowed = func(netip.Addr) bool { return true }
	c.RootCAs = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	c.Timeout = 2 * time.Second
	c.anyPort = true
	return c
}

func TestBootstrapEmbedded(t *testing.T) {
	c := New()
	for tld, want := range map[string]string{
		"com": "https://rdap.verisign.com/com/v1/",
		"net": "https://rdap.verisign.com/net/v1/",
		"id":  "https://rdap.pandi.id/rdap/",
	} {
		if got := c.Servers[tld]; got != want {
			t.Errorf("%s: %q, want %q", tld, got, want)
		}
	}
	for _, tld := range []string{"org", "dev", "app"} {
		if c.Servers[tld] == "" {
			t.Errorf("%s has no RDAP server", tld)
		}
	}
	for _, tld := range []string{"io", "co", "my"} {
		if c.Servers[tld] != "" {
			t.Errorf("%s unexpectedly has %q", tld, c.Servers[tld])
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string][2]string{
		"github.com":                      {"github.com", "github.com"},
		"https://WWW.Toko.co.id/path?q=1": {"www.toko.co.id", "toko.co.id"},
		"www.toko.co.id":                  {"www.toko.co.id", "toko.co.id"},
		"rifki.my.id":                     {"rifki.my.id", "rifki.my.id"},
		"a.b.rifki.my.id.":                {"a.b.rifki.my.id", "rifki.my.id"},
		"kominfo.go.id":                   {"kominfo.go.id", "kominfo.go.id"},
		"toko.github.io":                  {"toko.github.io", "github.io"},
		"bücher.de":                       {"xn--bcher-kva.de", "xn--bcher-kva.de"},
		"shop.contoh.unknowntld":          {"shop.contoh.unknowntld", "contoh.unknowntld"},
	} {
		host, domain, err := Normalize(in)
		if err != nil || host != want[0] || domain != want[1] {
			t.Errorf("Normalize(%q) = %q, %q, %v; want %q, %q", in, host, domain, err, want[0], want[1])
		}
	}
	for _, in := range []string{"", "localhost", "127.0.0.1", "[::1]", "co.id", "com", "toko.com:8080", "a b.com", "-x.com"} {
		if _, _, err := Normalize(in); !errors.Is(err, ErrInvalidDomain) {
			t.Errorf("Normalize(%q) error = %v, want ErrInvalidDomain", in, err)
		}
	}
}

func TestLookupExpirationAndRegistrar(t *testing.T) {
	var gotPath, gotAccept string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAccept = r.URL.Path, r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/rdap+json")
		fmt.Fprint(w, verisignLike)
	}))
	defer srv.Close()

	res, err := testClient(srv, "com").Lookup(context.Background(), "https://www.GitHub.com/")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/rdap/domain/github.com" || !strings.Contains(gotAccept, "application/rdap+json") {
		t.Errorf("request %q with Accept %q", gotPath, gotAccept)
	}
	// 18.20 UTC on 9 Oct is already 10 Oct in Jakarta.
	if res.Date.Format(time.DateOnly) != "2026-10-10" || res.Expiration.Format(time.RFC3339) != "2026-10-09T18:20:50Z" {
		t.Errorf("date %v, expiration %v", res.Date, res.Expiration)
	}
	if res.Registrar != "MarkMonitor Inc." || res.Domain != "github.com" || res.Server != "example.com" {
		t.Errorf("result %+v", res)
	}
}

func TestLookupErrors(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rdap/domain/hilang.com":
			http.Error(w, `{"errorCode":404}`, http.StatusNotFound)
		case "/rdap/domain/tanpatanggal.com":
			fmt.Fprint(w, `{"objectClassName":"domain","events":[{"eventAction":"registration","eventDate":"2020-01-01T00:00:00Z"}]}`)
		case "/rdap/domain/besar.com":
			fmt.Fprint(w, `{"objectClassName":"domain","remarks":"`+strings.Repeat("x", 1<<20)+`"}`)
		case "/rdap/domain/alih.com":
			http.Redirect(w, r, "http://example.com/rdap/domain/alih.com", http.StatusFound)
		case "/rdap/domain/rusak.com":
			fmt.Fprint(w, `<html>`)
		case "/rdap/domain/lambat.com":
			time.Sleep(3 * time.Second)
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := testClient(srv, "com")

	for host, want := range map[string]error{
		"hilang.com":       ErrNotFound,
		"tanpatanggal.com": ErrNoExpiration,
		"besar.com":        ErrBadResponse,
		"alih.com":         ErrBadResponse,
		"rusak.com":        ErrBadResponse,
		"lambat.com":       ErrUnreachable,
		"galat.com":        ErrUnreachable,
		"toko.io":          ErrNoRDAP,
		"localhost":        ErrInvalidDomain,
		"10.0.0.1":         ErrInvalidDomain,
	} {
		_, err := c.Lookup(context.Background(), host)
		if !errors.Is(err, want) {
			t.Errorf("%s: error %v, want %v", host, err, want)
		}
	}

	var noRDAP *NoRDAPError
	if _, err := c.Lookup(context.Background(), "toko.io"); !errors.As(err, &noRDAP) || noRDAP.TLD != "io" {
		t.Errorf("toko.io: %v", err)
	}
}

func TestLookupFollowsHTTPSRedirectOnly(t *testing.T) {
	var final *httptest.Server
	final = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rdap/domain/pindah.com" {
			http.Redirect(w, r, final.URL+"/other/domain/pindah.com", http.StatusMovedPermanently)
			return
		}
		fmt.Fprint(w, verisignLike)
	}))
	defer final.Close()
	c := testClient(final, "com")
	c.Servers["com"] = final.URL + "/rdap/" // the httptest certificate covers 127.0.0.1
	res, err := c.Lookup(context.Background(), "pindah.com")
	if err != nil {
		t.Fatal(err)
	}
	if res.Registrar != "MarkMonitor Inc." {
		t.Errorf("result %+v", res)
	}
}

func TestLookupRefusesPrivateAddressesAndOtherPorts(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, verisignLike)
	}))
	defer srv.Close()

	c := testClient(srv, "com")
	c.Allowed = func(a netip.Addr) bool { return !a.IsLoopback() }
	if _, err := c.Lookup(context.Background(), "github.com"); !errors.Is(err, ErrUnreachable) {
		t.Errorf("private address: %v", err)
	}

	c = testClient(srv, "com")
	c.anyPort = false
	if _, err := c.Lookup(context.Background(), "github.com"); !errors.Is(err, ErrBadResponse) {
		t.Errorf("port other than 443: %v", err)
	}
}

func TestRegistrarFallbacks(t *testing.T) {
	res, err := parseDomain([]byte(`{"events":[{"eventAction":"Expiration","eventDate":"2027-03-01"}],
		"entities":[{"roles":["registrant"],"entities":[{"roles":["registrar"],"handle":"PANDI-REG-1"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Registrar != "PANDI-REG-1" || res.Date.Format(time.DateOnly) != "2027-03-01" {
		t.Errorf("result %+v", res)
	}
}
