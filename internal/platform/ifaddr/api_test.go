package ifaddr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseAPIAddress(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{name: "plain", body: "2001:db8::1\n", want: "2001:db8::1", ok: true},
		{name: "crlf", body: "\r\n2001:db8::2\r\n", want: "2001:db8::2", ok: true},
		{name: "spaces", body: "  2001:db8::3  \nignored\n", want: "2001:db8::3", ok: true},
		{name: "ipv4", body: "192.0.2.1\n", ok: false},
		{name: "mapped", body: "::ffff:192.0.2.1\n", ok: false},
		{name: "empty", body: "\n\n", ok: false},
		{name: "junk", body: "hello\n", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAPIAddress(tt.body)
			if tt.ok {
				if err != nil {
					t.Fatal(err)
				}
				if got.String() != tt.want {
					t.Fatalf("parseAPIAddress() = %s, want %s", got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseAPIAddress() = %s, want error", got)
			}
		})
	}
}

func TestFetchIPFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "curl/8.0.0" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write([]byte("2001:db8::10\n"))
	}))
	defer srv.Close()

	got, err := fetchIPFromURL(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "2001:db8::10" {
		t.Fatalf("fetch = %s", got)
	}

	infos, err := GetIPv6FromAPIsSequential(context.Background(), []string{srv.URL}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].IP.String() != "2001:db8::10" || !infos[0].IsCandidate {
		t.Fatalf("sequential = %+v", infos)
	}
}

func TestFetchIPFromURLCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
			_, _ = w.Write([]byte("2001:db8::1\n"))
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := fetchIPFromURL(ctx, srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("canceled fetch returned an address")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestGetIPv6FromAPIsNoURLs(t *testing.T) {
	if _, err := GetIPv6FromAPIs(context.Background(), nil, true); err == nil {
		t.Fatal("concurrent API accepted an empty list")
	}
	if _, err := GetIPv6FromAPIsSequential(context.Background(), nil, nil); err == nil {
		t.Fatal("sequential API accepted an empty list")
	}
}
