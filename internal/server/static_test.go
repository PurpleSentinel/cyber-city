// SPDX-License-Identifier: MIT

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	auth, err := NewAuth()
	if err != nil {
		t.Fatal(err)
	}
	static := fstest.MapFS{
		"index.html":         {Data: []byte("<title>x</title>")},
		"assets/index-a.js":  {Data: []byte("console.log(1)")},
		"assets/index-a.css": {Data: []byte("body{}")},
	}
	h := Handler(NewHub(nil, nil, DefaultHubOptions()), static, auth)

	cases := []struct {
		path    string
		status  int
		noStore bool
	}{
		{"/", http.StatusOK, true},
		{"/index.html", http.StatusMovedPermanently, true}, // FileServer canonicalises to /
		{"/assets/index-a.js", http.StatusOK, false},
		{"/assets/", http.StatusNotFound, false}, // no directory listing
		{"/missing", http.StatusNotFound, false},
		{"/auth?token=nope", http.StatusUnauthorized, true},
		{"/api/session", http.StatusUnauthorized, true},
		{"/ws", http.StatusUnauthorized, false},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.path, rec.Code, tc.status)
		}
		hdr := rec.Header()
		if hdr.Get("Content-Security-Policy") != contentSecurityPolicy {
			t.Errorf("%s: CSP %q", tc.path, hdr.Get("Content-Security-Policy"))
		}
		if hdr.Get("X-Content-Type-Options") != "nosniff" || hdr.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: missing nosniff/no-referrer: %v", tc.path, hdr)
		}
		if got := hdr.Get("Cache-Control") == "no-store"; got != tc.noStore {
			t.Errorf("%s: no-store=%v, want %v", tc.path, got, tc.noStore)
		}
	}
}
