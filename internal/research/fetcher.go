// Package research contains the untrusted-web ingestion boundary for Qwen.
package research

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	maxDownloadBytes = 2 << 20
	maxTextBytes     = 1500
	maxRedirects     = 3
)

var tagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

// Result is deliberately plain text. Callers must treat Text as hostile
// reference material, not as instructions or trusted HTML.
type Result struct {
	URL         string
	ContentType string
	Text        string
}

func blockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast() ||
		(ip.To4() != nil && v4InRange(ip, net.ParseIP("100.64.0.0"), 10)) ||
		(ip.To4() != nil && v4InRange(ip, net.ParseIP("198.18.0.0"), 15)) ||
		(ip.To4() == nil && ip.IsPrivate())
}

func v4InRange(ip, base net.IP, bits int) bool {
	ip, base = ip.To4(), base.To4()
	if ip == nil || base == nil {
		return false
	}
	mask := net.CIDRMask(bits, 32)
	return ip.Mask(mask).Equal(base.Mask(mask))
}

func validateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return nil, errors.New("only http(s) URLs without userinfo are allowed")
	}
	port := u.Port()
	if port != "" && port != "80" && port != "443" {
		return nil, errors.New("only ports 80 and 443 are allowed")
	}
	return u, nil
}

// Fetch retrieves one public HTTP(S) document. Every connection resolves and
// validates its destination immediately before dialing, and redirects are
// validated independently.
func Fetch(ctx context.Context, raw string) (Result, error) {
	first, err := validateURL(raw)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resolver := net.Resolver{}
	transport := &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := resolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if blockedIP(ip) {
					continue
				}
				conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
			}
			return nil, errors.New("destination resolves only to blocked addresses")
		},
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("too many redirects")
		}
		_, err := validateURL(req.URL.String())
		return err
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, first.String(), nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", "ForgeGrid-research/1")
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("research source returned HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType != "text/html" && contentType != "text/plain" {
		return Result{}, errors.New("research source has unsupported content type")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return Result{}, err
	}
	if len(body) > maxDownloadBytes {
		return Result{}, errors.New("research source exceeds 2 MiB")
	}
	text := strings.TrimSpace(tagPattern.ReplaceAllString(string(body), " "))
	if len(text) > maxTextBytes {
		text = text[:maxTextBytes]
	}
	return Result{URL: resp.Request.URL.String(), ContentType: contentType, Text: text}, nil
}
