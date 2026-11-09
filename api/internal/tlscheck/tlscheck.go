// Package tlscheck reads the certificate a public HTTPS server presents, so the
// expiry date comes from the server itself instead of being typed by hand.
//
// The host is user input, so a lookup must never become a way to probe the
// network the API runs in: the name has to resolve only to public addresses,
// the port is fixed to 443, and the connection goes to the address that was
// checked, not to a second DNS answer.
package tlscheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"time"
)

var (
	ErrInvalidHost   = errors.New("alamat situs tidak valid. Tulis nama domain, misalnya tokokuemawar.id")
	ErrBlockedHost   = errors.New("host ini mengarah ke alamat jaringan lokal atau privat dan tidak dapat diperiksa")
	ErrNotFound      = errors.New("host tidak ditemukan di DNS")
	ErrUnreachable   = errors.New("server tidak dapat dihubungi di port 443")
	ErrHandshake     = errors.New("server tidak menyelesaikan jabat tangan TLS")
	ErrNoCertificate = errors.New("server tidak mengirim sertifikat")
)

// Result describes the leaf certificate a server presented.
type Result struct {
	Host       string    `json:"host"`
	Address    string    `json:"alamat"`
	Issuer     string    `json:"penerbit"`
	Subject    string    `json:"subjek"`
	NotBefore  time.Time `json:"berlaku_sejak"`
	NotAfter   time.Time `json:"berakhir"`
	ChainValid bool      `json:"rantai_valid"`
	ChainError string    `json:"galat_rantai,omitempty"`
	CheckedAt  time.Time `json:"dicek_pada"`
}

// Checker looks up certificates. The zero value is not usable; call New.
type Checker struct {
	// Lookup resolves a host name. Tests replace it; New uses the system resolver.
	Lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	// Allowed decides which resolved addresses may be contacted. New uses Public.
	Allowed func(netip.Addr) bool
	// Roots verifies the chain. Nil means the system roots.
	Roots   *x509.CertPool
	Port    string
	Timeout time.Duration
	Now     func() time.Time
}

// New returns a Checker for public hosts on port 443 with a 5 second limit.
func New() *Checker {
	return &Checker{
		Lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		Allowed: Public,
		Port:    "443",
		Timeout: 5 * time.Second,
		Now:     time.Now,
	}
}

// NormalizeHost turns what a user typed ("https://TokoKu.id/path") into a bare,
// lower-case host name ("tokoku.id"). IP literals, ports, single-label names such
// as "localhost" or a Docker service name, and anything that is not a plain ASCII
// host name are refused.
func NormalizeHost(input string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(input))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexAny(h, "/?#"); i >= 0 {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")
	if h == "" || len(h) > 253 || strings.ContainsAny(h, ":@[] ") {
		return "", ErrInvalidHost
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return "", ErrInvalidHost
	}
	labels := strings.Split(h, ".")
	if len(labels) < 2 {
		return "", ErrInvalidHost
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return "", ErrInvalidHost
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", ErrInvalidHost
			}
		}
	}
	return h, nil
}

// Special-purpose ranges that are neither private nor loopback but must not be
// reached either: shared address space (carrier NAT), benchmarking,
// documentation, reserved, NAT64 local use and the IPv6 discard prefix.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// Public reports whether an address belongs to the public internet. Loopback,
// private (including Docker networks and cloud metadata at 169.254.169.254),
// link-local, multicast and the special ranges above are refused.
func Public(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || !a.IsGlobalUnicast() || a.IsPrivate() {
		return false
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// Check connects to host on the configured port and returns its certificate.
// A certificate that has expired or does not verify is still returned, with
// ChainValid false, because the date is exactly what the user needs to see.
func (c *Checker) Check(ctx context.Context, rawHost string) (*Result, error) {
	host, err := NormalizeHost(rawHost)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	addrs, err := c.Lookup(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil, ErrNotFound
	}
	// Refuse the host if any answer is local, so a name cannot mix a public and
	// a private address and hope the private one is dialled.
	for _, a := range addrs {
		if !c.Allowed(a) {
			return nil, ErrBlockedHost
		}
	}
	addr := preferIPv4(addrs)

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(addr.String(), c.Port))
	if err != nil {
		return nil, ErrUnreachable
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	// Verification is skipped during the handshake on purpose: an expired or
	// misconfigured certificate must still be read. The chain is verified
	// separately below and reported as ChainValid.
	tc := tls.Client(conn, &tls.Config{ServerName: host, InsecureSkipVerify: true}) //nolint:gosec // verified manually below
	if err := tc.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	certs := tc.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, ErrNoCertificate
	}
	leaf := certs[0]

	now := c.Now()
	res := &Result{
		Host:      host,
		Address:   addr.String(),
		Issuer:    issuerName(leaf),
		Subject:   leaf.Subject.CommonName,
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
		CheckedAt: now,
	}
	intermediates := x509.NewCertPool()
	for _, ic := range certs[1:] {
		intermediates.AddCert(ic)
	}
	_, verr := leaf.Verify(x509.VerifyOptions{
		DNSName:       host,
		Intermediates: intermediates,
		Roots:         c.Roots,
		CurrentTime:   now,
	})
	res.ChainValid = verr == nil
	if verr != nil {
		res.ChainError = verr.Error()
	}
	return res, nil
}

// issuerName names the certificate's issuer from its organisation and common
// name without repeating a word: "Sectigo Limited" and "Sectigo Public Server
// Authentication CA DV E36" give the common name alone, "Let's Encrypt" and
// "R12" give "Let's Encrypt · R12".
func issuerName(cert *x509.Certificate) string {
	cn := strings.TrimSpace(cert.Issuer.CommonName)
	org := ""
	if len(cert.Issuer.Organization) > 0 {
		org = strings.TrimSpace(cert.Issuer.Organization[0])
	}
	switch {
	case org == "":
		return cn
	case cn == "" || cn == org:
		return org
	}
	if first := strings.Fields(org); len(first) > 0 && slices.Contains(strings.Fields(strings.ToLower(cn)), strings.ToLower(first[0])) {
		return cn
	}
	return org + " · " + cn
}

// preferIPv4 picks an IPv4 address when there is one: Docker bridge networks
// often have no IPv6 route out.
func preferIPv4(addrs []netip.Addr) netip.Addr {
	for _, a := range addrs {
		if a.Unmap().Is4() {
			return a.Unmap()
		}
	}
	return addrs[0]
}
