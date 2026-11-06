package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitFQDN(t *testing.T) {
	tests := []struct {
		in   string
		rr   string
		zone string
		ok   bool
	}{
		{in: "example.com", rr: "@", zone: "example.com", ok: true},
		{in: "Example.COM.", rr: "@", zone: "example.com", ok: true},
		{in: "www.example.com", rr: "www", zone: "example.com", ok: true},
		{in: "www.example.com.cn", rr: "www", zone: "example.com.cn", ok: true},
		{in: "example.com.cn", rr: "example", zone: "com.cn", ok: true},
		{in: "", ok: false},
		{in: "localhost", ok: false},
		{in: "www..example.com", ok: false},
		{in: "a b.com", ok: false},
	}
	for _, tt := range tests {
		rr, zone, err := splitFQDN(tt.in)
		if tt.ok {
			if err != nil {
				t.Errorf("splitFQDN(%q) error = %v", tt.in, err)
				continue
			}
			if rr != tt.rr || zone != tt.zone {
				t.Errorf("splitFQDN(%q) = %q, %q; want %q, %q", tt.in, rr, zone, tt.rr, tt.zone)
			}
		} else if err == nil {
			t.Errorf("splitFQDN(%q) = %q, %q; want error", tt.in, rr, zone)
		}
	}
}

func TestRecordFQDN(t *testing.T) {
	apex := &RecordConfig{Name: "@", Zone: "example.com"}
	if got := RecordFQDN(apex); got != "example.com" {
		t.Errorf("RecordFQDN(apex) = %q", got)
	}
	host := &RecordConfig{Name: "www", Zone: "example.com"}
	if got := RecordFQDN(host); got != "www.example.com" {
		t.Errorf("RecordFQDN(host) = %q", got)
	}
}

func TestParseConfig(t *testing.T) {
	raw := []byte(`{
	  "interface": "vtnet0",
	  "fallback_urls": ["https://ipv6.icanhazip.com"],
	  "proxy": "socks5://127.0.0.1:1080",
	  "cloudflare": {
	    "token": "cf-token",
	    "records": [
	      {"name": "example.com", "ttl": 180, "proxied": true, "use_proxy": true},
	      {"name": "www.example.com", "zone_id": "zone123"}
	    ]
	  },
	  "aliyun": {
	    "access_key_id": "ak",
	    "access_key_secret": "secret",
	    "records": [
	      {"name": "www.example.cn", "ttl": 600, "use_proxy": true}
	    ]
	  }
	}`)

	cfg, err := parseConfig(raw)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.IPSource.Interface != "vtnet0" || cfg.Proxy == "" {
		t.Fatalf("ip source or proxy not loaded: %+v", cfg)
	}
	if len(cfg.Records) != 3 {
		t.Fatalf("records = %d, want 3", len(cfg.Records))
	}

	apex := cfg.Records[0]
	if apex.Provider != "cloudflare" || apex.Name != "@" || apex.Zone != "example.com" {
		t.Errorf("apex = %+v", apex)
	}
	if apex.APIToken != "cf-token" || !apex.Proxied || !apex.UseProxy || apex.TTL != 180 {
		t.Errorf("apex fields = %+v", apex)
	}

	www := cfg.Records[1]
	if www.Name != "www" || www.Zone != "example.com" || www.ZoneID != "zone123" || www.TTL != 0 {
		t.Errorf("www = %+v", www)
	}

	ali := cfg.Records[2]
	if ali.Provider != "aliyun" || ali.Name != "www" || ali.Zone != "example.cn" {
		t.Errorf("aliyun = %+v", ali)
	}
	if ali.AccessKeyID != "ak" || ali.AccessKeySecret != "secret" || !ali.UseProxy || ali.TTL != 600 {
		t.Errorf("aliyun fields = %+v", ali)
	}
	if GetRecordTTL(&www) != 180 || GetRecordTTL(&ali) != 600 {
		t.Errorf("default TTL cloudflare=%d aliyun=%d", GetRecordTTL(&www), GetRecordTTL(&ali))
	}
	if GetRecordProxy(cfg, &ali) != cfg.Proxy {
		t.Errorf("aliyun proxy = %q", GetRecordProxy(cfg, &ali))
	}
}

func TestParseConfigRejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "old env", raw: `{"env":{},"interface":"eth0","cloudflare":{"token":"t","records":[{"name":"example.com"}]}}`, want: "unknown field"},
		{name: "no source", raw: `{"cloudflare":{"token":"t","records":[{"name":"example.com"}]}}`, want: "interface"},
		{name: "empty cloudflare records", raw: `{"interface":"eth0","cloudflare":{"token":"t","records":[]}}`, want: "cloudflare.records"},
		{name: "missing token", raw: `{"interface":"eth0","cloudflare":{"token":"","records":[{"name":"example.com"}]}}`, want: "cloudflare.token"},
		{name: "missing aliyun secret", raw: `{"interface":"eth0","aliyun":{"access_key_id":"ak","access_key_secret":"","records":[{"name":"example.cn"}]}}`, want: "access_key_secret"},
		{name: "proxy without url", raw: `{"interface":"eth0","cloudflare":{"token":"t","records":[{"name":"example.com","use_proxy":true}]}}`, want: "use_proxy"},
		{name: "bad proxy scheme", raw: `{"interface":"eth0","proxy":"ftp://127.0.0.1","cloudflare":{"token":"t","records":[{"name":"example.com"}]}}`, want: "unsupported proxy"},
		{name: "one label", raw: `{"interface":"eth0","cloudflare":{"token":"t","records":[{"name":"localhost"}]}}`, want: "two labels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig([]byte(tt.raw))
			if err == nil {
				t.Fatal("parseConfig() error = nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestParseCacheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.lastip")

	if got := ReadLastIP(path); got != "" {
		t.Errorf("missing cache = %q", got)
	}

	history := "# comment\n\n2026-05-01T10:30:00Z 2001:db8::1\n2026-05-02T10:30:00Z 2001:db8::2\n"
	if err := os.WriteFile(path, []byte(history), 0600); err != nil {
		t.Fatal(err)
	}
	if got := ReadLastIP(path); got != "2001:db8::2" {
		t.Errorf("history last IP = %q", got)
	}

	if err := WriteLastIP(path, "2001:db8::9"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "2001:db8::9\n" {
		t.Errorf("written cache = %q", body)
	}
	if got := ReadLastIP(path); got != "2001:db8::9" {
		t.Errorf("bare IP = %q", got)
	}
}
