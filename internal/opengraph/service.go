package opengraph

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

// Metadata represents OpenGraph metadata for a URL.
type Metadata struct {
	URL         string  `json:"url"`
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Image       string  `json:"image,omitempty"`
	SiteName    string  `json:"site_name,omitempty"`
	Type        string  `json:"type,omitempty"`
	Favicon     string  `json:"favicon,omitempty"`
}

// cacheEntry holds cached metadata with expiration.
type cacheEntry struct {
	metadata  *Metadata
	expiresAt time.Time
	err       error
}

// Service fetches OpenGraph metadata from URLs.
type Service struct {
	client    *http.Client
	cache     map[string]cacheEntry
	cacheMu   sync.RWMutex
	cacheTTL  time.Duration
	userAgent string
}

// blockedHosts contains internal Docker service hostnames that must not be accessed.
var blockedHosts = map[string]bool{
	"localhost":   true,
	"postgres":    true,
	"redis":       true,
	"livekit":     true,
	"caddy":       true,
	"localstack":  true,
	"redoubt-api": true,
	"prometheus":  true,
	"grafana":     true,
}

// isPrivateIP returns true if the IP is in a private, loopback, link-local, or
// cloud metadata range that should not be reachable via user-supplied URLs.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{parseCIDR("10.0.0.0/8")},
		{parseCIDR("172.16.0.0/12")},
		{parseCIDR("192.168.0.0/16")},
		{parseCIDR("127.0.0.0/8")},
		{parseCIDR("169.254.0.0/16")},   // link-local / cloud metadata
		{parseCIDR("::1/128")},           // IPv6 loopback
		{parseCIDR("fc00::/7")},          // IPv6 unique local
		{parseCIDR("fe80::/10")},         // IPv6 link-local
		{parseCIDR("0.0.0.0/8")},        // "this" network
		{parseCIDR("100.64.0.0/10")},     // shared address space (CGN)
		{parseCIDR("192.0.0.0/24")},      // IETF protocol assignments
		{parseCIDR("198.18.0.0/15")},     // benchmarking
		{parseCIDR("240.0.0.0/4")},       // reserved
	}

	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

func parseCIDR(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		panic("invalid CIDR: " + s)
	}
	return network
}

// safeDialContext wraps the default dialer to reject connections to private IPs.
// This prevents SSRF attacks by validating resolved addresses before connecting.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// Resolve the hostname to IP addresses
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed: %w", err)
	}

	// Check all resolved IPs
	for _, ip := range ips {
		if isPrivateIP(ip.IP) {
			return nil, fmt.Errorf("blocked: %s resolves to private IP %s", host, ip.IP)
		}
	}

	// Connect using the validated address
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// NewService creates a new OpenGraph service.
func NewService() *Service {
	return &Service{
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: safeDialContext,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return http.ErrUseLastResponse
				}
				// Validate redirect targets against blocked hosts
				if blockedHosts[strings.ToLower(req.URL.Hostname())] {
					return fmt.Errorf("blocked: redirect to internal host %s", req.URL.Hostname())
				}
				return nil
			},
		},
		cache:     make(map[string]cacheEntry),
		cacheTTL:  15 * time.Minute,
		userAgent: "Mozilla/5.0 (compatible; Redoubt/1.0; +https://redoubt.chat)",
	}
}

// Fetch retrieves OpenGraph metadata for a URL.
func (s *Service) Fetch(ctx context.Context, rawURL string) (*Metadata, error) {
	// Validate and normalize URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	// Only allow http/https
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return &Metadata{URL: rawURL}, nil
	}

	// Block requests to internal/private hostnames
	hostname := strings.ToLower(parsedURL.Hostname())
	if blockedHosts[hostname] {
		return &Metadata{URL: rawURL}, nil
	}

	// Block requests where the hostname is a raw IP in a private range
	if ip := net.ParseIP(hostname); ip != nil && isPrivateIP(ip) {
		return &Metadata{URL: rawURL}, nil
	}

	// Check cache
	s.cacheMu.RLock()
	if entry, ok := s.cache[rawURL]; ok && time.Now().Before(entry.expiresAt) {
		s.cacheMu.RUnlock()
		return entry.metadata, entry.err
	}
	s.cacheMu.RUnlock()

	// Fetch metadata
	metadata, err := s.fetchFromURL(ctx, rawURL, parsedURL)

	// Cache result (including errors)
	s.cacheMu.Lock()
	s.cache[rawURL] = cacheEntry{
		metadata:  metadata,
		expiresAt: time.Now().Add(s.cacheTTL),
		err:       err,
	}
	s.cacheMu.Unlock()

	return metadata, err
}

func (s *Service) fetchFromURL(ctx context.Context, rawURL string, parsedURL *url.URL) (*Metadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Metadata{URL: rawURL}, nil
	}

	// Limit body size to 1MB
	body := io.LimitReader(resp.Body, 1024*1024)

	metadata := s.parseHTML(body, rawURL, parsedURL)
	return metadata, nil
}

func (s *Service) parseHTML(r io.Reader, rawURL string, parsedURL *url.URL) *Metadata {
	doc, err := html.Parse(r)
	if err != nil {
		return &Metadata{URL: rawURL}
	}

	metadata := &Metadata{URL: rawURL}

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "meta":
				s.parseMeta(n, metadata)
			case "title":
				if metadata.Title == "" && n.FirstChild != nil {
					metadata.Title = strings.TrimSpace(n.FirstChild.Data)
				}
			case "link":
				s.parseLink(n, metadata, parsedURL)
			}
		}

		// Stop traversing after head
		if n.Type == html.ElementNode && n.Data == "body" {
			return
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)

	// Set default favicon if none found
	if metadata.Favicon == "" {
		metadata.Favicon = parsedURL.Scheme + "://" + parsedURL.Host + "/favicon.ico"
	}

	return metadata
}

func (s *Service) parseMeta(n *html.Node, metadata *Metadata) {
	var property, name, content string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "property":
			property = attr.Val
		case "name":
			name = attr.Val
		case "content":
			content = attr.Val
		}
	}

	// OpenGraph properties
	switch property {
	case "og:title":
		metadata.Title = content
	case "og:description":
		metadata.Description = content
	case "og:image":
		metadata.Image = content
	case "og:site_name":
		metadata.SiteName = content
	case "og:type":
		metadata.Type = content
	}

	// Fallback to standard meta tags
	switch name {
	case "description":
		if metadata.Description == "" {
			metadata.Description = content
		}
	case "twitter:title":
		if metadata.Title == "" {
			metadata.Title = content
		}
	case "twitter:description":
		if metadata.Description == "" {
			metadata.Description = content
		}
	case "twitter:image":
		if metadata.Image == "" {
			metadata.Image = content
		}
	}
}

func (s *Service) parseLink(n *html.Node, metadata *Metadata, baseURL *url.URL) {
	var rel, href string
	for _, attr := range n.Attr {
		switch attr.Key {
		case "rel":
			rel = attr.Val
		case "href":
			href = attr.Val
		}
	}

	// Look for favicon
	if strings.Contains(rel, "icon") && href != "" && metadata.Favicon == "" {
		if strings.HasPrefix(href, "//") {
			metadata.Favicon = baseURL.Scheme + ":" + href
		} else if strings.HasPrefix(href, "/") {
			metadata.Favicon = baseURL.Scheme + "://" + baseURL.Host + href
		} else if strings.HasPrefix(href, "http") {
			metadata.Favicon = href
		}
	}
}

// CleanupCache removes expired entries from the cache.
func (s *Service) CleanupCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	now := time.Now()
	for url, entry := range s.cache {
		if now.After(entry.expiresAt) {
			delete(s.cache, url)
		}
	}
}

// ExtractURLs extracts URLs from text content.
func ExtractURLs(text string) []string {
	urlRegex := regexp.MustCompile(`https?://[^\s<>\[\]()]+`)
	matches := urlRegex.FindAllString(text, -1)

	// Deduplicate
	seen := make(map[string]bool)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		// Clean trailing punctuation
		m = strings.TrimRight(m, ".,;:!?\"')")
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}
