// Package rdap reads a domain's expiry date and registrar from its registry
// over RDAP (RFC 9082, RFC 9083), so a domain's date no longer has to be typed
// by hand.
//
// The registry for each top-level domain comes from IANA's RDAP bootstrap file
// (RFC 9224). A snapshot is embedded as dns.json; the copy in this package was
// published by IANA on 2026-10-28 ("publication": "2026-10-28T22:00:04Z") and
// downloaded from https://data.iana.org/rdap/dns.json on 2026-11-10.
//
// The domain is user input, but it only ever ends up in the URL path: the
// server is chosen from the bootstrap file. Redirects are still followed with
// care, at most two and only to HTTPS on port 443 at hosts that resolve to
// public addresses, using the same guard as internal/tlscheck.
package rdap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"

	"github.com/mrramdn/tenggat/api/internal/expiry"
	"github.com/mrramdn/tenggat/api/internal/tlscheck"
)

//go:embed dns.json
var bootstrapJSON []byte

var (
	ErrInvalidDomain = errors.New("nama domain tidak valid. Tulis nama domain, misalnya mrramdn.com")
	ErrNoRDAP        = errors.New("registri domain ini belum menyediakan RDAP")
	ErrNotFound      = errors.New("domain tidak ditemukan di registri")
	ErrNoExpiration  = errors.New("registri tidak mencantumkan tanggal berakhir domain ini")
	ErrUnreachable   = errors.New("registri tidak dapat dihubungi")
	ErrBadResponse   = errors.New("jawaban registri tidak dapat dibaca")
)

// NoRDAPError says which top-level domain has no RDAP server. It matches ErrNoRDAP.
type NoRDAPError struct{ TLD string }

func (e *NoRDAPError) Error() string { return "registri ." + e.TLD + " belum menyediakan RDAP" }

// Is makes errors.Is(err, ErrNoRDAP) true.
func (e *NoRDAPError) Is(target error) bool { return target == ErrNoRDAP }

// Result is what the registry says about one domain.
type Result struct {
	Domain     string    `json:"domain"`     // the registrable domain that was asked for, e.g. toko.co.id
	Expiration time.Time `json:"expiration"` // the instant of the "expiration" event
	Date       time.Time `json:"date"`       // its calendar date in Jakarta, as midnight UTC
	Registrar  string    `json:"registrar"`  // the registrar's name, empty when the registry gives none
	Server     string    `json:"server"`     // the host name of the RDAP server that answered
}

// Client looks up domains. The zero value is not usable; call New.
type Client struct {
	// Servers maps a top-level domain (without dot) to its RDAP base URL, ending in "/".
	Servers map[string]string
	// Resolve resolves a host name. Tests replace it; New uses the system resolver.
	Resolve func(ctx context.Context, host string) ([]netip.Addr, error)
	// Allowed decides which resolved addresses may be contacted. New uses tlscheck.Public.
	Allowed func(netip.Addr) bool
	// RootCAs verifies the server. Nil means the system roots.
	RootCAs *x509.CertPool
	Timeout time.Duration
	MaxBody int64

	anyPort bool // tests: allow the random port of an httptest server
}

// New returns a Client for the embedded bootstrap file with an 8 second limit
// and a 1 MiB body limit.
func New() *Client {
	servers, err := ParseBootstrap(bootstrapJSON)
	if err != nil {
		panic("rdap: embedded bootstrap file: " + err.Error())
	}
	return &Client{
		Servers: servers,
		Resolve: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		Allowed: tlscheck.Public,
		Timeout: 8 * time.Second,
		MaxBody: 1 << 20,
	}
}

// ParseBootstrap reads an IANA RDAP bootstrap file into a TLD → base URL map,
// keeping the first HTTPS URL of each service.
func ParseBootstrap(data []byte) (map[string]string, error) {
	var file struct {
		Publication string       `json:"publication"`
		Services    [][][]string `json:"services"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	servers := map[string]string{}
	for _, svc := range file.Services {
		if len(svc) != 2 {
			continue
		}
		base := ""
		for _, u := range svc[1] {
			if strings.HasPrefix(u, "https://") {
				base = u
				break
			}
		}
		if base == "" {
			continue
		}
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		for _, tld := range svc[0] {
			servers[strings.ToLower(tld)] = base
		}
	}
	if len(servers) == 0 {
		return nil, errors.New("no services")
	}
	return servers, nil
}

// Normalize turns what a user typed ("https://www.Toko.co.id/path") into the
// lower-case ASCII host ("www.toko.co.id") and the domain registered at the
// registry ("toko.co.id"). Internationalised names are converted to their
// xn-- form. IP addresses, single labels and public suffixes are refused.
func Normalize(input string) (host, domain string, err error) {
	h := strings.TrimSpace(input)
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimSuffix(strings.ToLower(h), ".")
	if h == "" {
		return "", "", ErrInvalidDomain
	}
	if _, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
		return "", "", ErrInvalidDomain
	}
	ascii, err := idna.Lookup.ToASCII(h)
	if err != nil {
		return "", "", ErrInvalidDomain
	}
	host, err = tlscheck.NormalizeHost(ascii)
	if err != nil {
		return "", "", ErrInvalidDomain
	}
	domain, err = Registrable(host)
	if err != nil {
		return "", "", err
	}
	return host, domain, nil
}

// ToASCII returns the xn-- form of a host typed with non-ASCII letters, and
// the input unchanged when it is ASCII already or cannot be converted.
func ToASCII(input string) string {
	for _, c := range input {
		if c >= 0x80 {
			h, _, err := Normalize(input)
			if err != nil {
				return input
			}
			return h
		}
	}
	return input
}

// Registrable reduces a host to the domain a registry knows: one label below
// the ICANN public suffix (www.toko.co.id → toko.co.id). Suffixes from the
// private section of the Public Suffix List, such as github.io, are skipped,
// because the registry only knows github.io itself.
func Registrable(host string) (string, error) {
	suffix, icann := publicsuffix.PublicSuffix(host)
	for !icann {
		i := strings.IndexByte(suffix, '.')
		if i < 0 {
			break // a TLD the list does not know: the default rule "*" applies
		}
		suffix, icann = publicsuffix.PublicSuffix(suffix[i+1:])
	}
	if host == suffix || !strings.HasSuffix(host, "."+suffix) {
		return "", ErrInvalidDomain
	}
	rest := strings.TrimSuffix(host, "."+suffix)
	return rest[strings.LastIndexByte(rest, '.')+1:] + "." + suffix, nil
}

// TLD returns the last label of a host.
func TLD(host string) string { return host[strings.LastIndexByte(host, '.')+1:] }

// Lookup asks the registry of host's top-level domain about host's
// registrable domain.
func (c *Client) Lookup(ctx context.Context, rawHost string) (*Result, error) {
	_, domain, err := Normalize(rawHost)
	if err != nil {
		return nil, err
	}
	tld := TLD(domain)
	base, ok := c.Servers[tld]
	if !ok {
		return nil, &NoRDAPError{TLD: tld}
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"domain/"+domain, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	req.Header.Set("User-Agent", "Tenggat (+https://tenggat.mrramdn.com)")
	if err := c.allowedURL(req.URL); err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		var known *lookupError
		if errors.As(err, &known) {
			return nil, known.err
		}
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: status %d", ErrUnreachable, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.MaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if int64(len(body)) > c.MaxBody {
		return nil, fmt.Errorf("%w: lebih dari %d byte", ErrBadResponse, c.MaxBody)
	}
	res, err := parseDomain(body)
	if err != nil {
		return nil, err
	}
	res.Domain = domain
	res.Server = resp.Request.URL.Hostname()
	return res, nil
}

// lookupError carries one of the package errors through net/http, which wraps
// whatever a dialer or redirect check returns.
type lookupError struct{ err error }

func (e *lookupError) Error() string { return e.err.Error() }
func (e *lookupError) Unwrap() error { return e.err }

// allowedURL accepts only HTTPS on port 443.
func (c *Client) allowedURL(u *url.URL) error {
	if u.Scheme != "https" {
		return &lookupError{fmt.Errorf("%w: pengalihan ke alamat bukan HTTPS ditolak", ErrBadResponse)}
	}
	if p := u.Port(); p != "" && p != "443" && !c.anyPort {
		return &lookupError{fmt.Errorf("%w: port %s ditolak", ErrBadResponse, p)}
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil, // a proxy would bypass the address check
			DialContext:           c.dial,
			TLSClientConfig:       &tls.Config{RootCAs: c.RootCAs, MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout:   c.Timeout,
			ResponseHeaderTimeout: c.Timeout,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 2 {
				return &lookupError{fmt.Errorf("%w: terlalu banyak pengalihan", ErrBadResponse)}
			}
			return c.allowedURL(req.URL)
		},
	}
}

// dial resolves the host itself, refuses it if any answer is not public, and
// connects to the address it checked.
func (c *Client) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	addrs, err := c.Resolve(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil, &lookupError{fmt.Errorf("%w: %s tidak ditemukan di DNS", ErrUnreachable, host)}
	}
	for _, a := range addrs {
		if !c.Allowed(a) {
			return nil, &lookupError{fmt.Errorf("%w: %s mengarah ke alamat privat", ErrUnreachable, host)}
		}
	}
	var d net.Dialer
	var lastErr error
	for _, a := range addrs {
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(a.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// The parts of an RDAP domain object (RFC 9083 section 5.3) Tenggat reads.
type domainObject struct {
	Events   []event  `json:"events"`
	Entities []entity `json:"entities"`
}

type event struct {
	Action string `json:"eventAction"`
	Date   string `json:"eventDate"`
}

type entity struct {
	Handle   string          `json:"handle"`
	Roles    []string        `json:"roles"`
	VCard    json.RawMessage `json:"vcardArray"`
	Entities []entity        `json:"entities"`
}

func parseDomain(body []byte) (*Result, error) {
	var obj domainObject
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	res := &Result{Registrar: registrar(obj.Entities)}
	for _, e := range obj.Events {
		if !strings.EqualFold(e.Action, "expiration") {
			continue
		}
		t, ok := parseTime(e.Date)
		if !ok {
			return nil, fmt.Errorf("%w: tanggal %q", ErrBadResponse, e.Date)
		}
		res.Expiration = t
		res.Date = expiry.DateOf(t)
		return res, nil
	}
	return nil, ErrNoExpiration
}

func parseTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", time.DateOnly} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// registrar returns the fn of the entity with role "registrar", looking one
// level into nested entities too.
func registrar(entities []entity) string {
	for _, e := range entities {
		for _, r := range e.Roles {
			if strings.EqualFold(r, "registrar") {
				if fn := vcardFN(e.VCard); fn != "" {
					return fn
				}
				return e.Handle
			}
		}
	}
	for _, e := range entities {
		if name := registrar(e.Entities); name != "" {
			return name
		}
	}
	return ""
}

// vcardFN reads the fn property of a jCard (RFC 7095):
// ["vcard", [["version", {}, "text", "4.0"], ["fn", {}, "text", "Name"], ...]].
func vcardFN(raw json.RawMessage) string {
	var card []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &card) != nil || len(card) < 2 {
		return ""
	}
	var props [][]json.RawMessage
	if json.Unmarshal(card[1], &props) != nil {
		return ""
	}
	for _, p := range props {
		if len(p) < 4 {
			continue
		}
		var name, value string
		if json.Unmarshal(p[0], &name) == nil && strings.EqualFold(name, "fn") && json.Unmarshal(p[3], &value) == nil {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
