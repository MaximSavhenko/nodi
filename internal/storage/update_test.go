package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestStableCLIRelease(t *testing.T) {
	for _, body := range []string{`{"tag_name":"v0.4.0","draft":false,"prerelease":false}`, `{"tag_name":"v0.4.0","draft":true}`, `{"tag_name":"v0.4.0","prerelease":true}`, `{"tag_name":"0.4.0"}`, `{"tag_name":"v0.4.0-beta"}`, `invalid JSON`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.Header.Get("Authorization") != "" {
					t.Error("update request mutated state or sent a credential")
				}
				w.Write([]byte(body))
			}))
			defer s.Close()
			version, e := latestCLIRelease(context.Background(), s.Client(), s.URL)
			if body == `{"tag_name":"v0.4.0","draft":false,"prerelease":false}` {
				if e != nil || version != "0.4.0" {
					t.Fatal(version, e)
				}
			} else if e == nil {
				t.Fatal("invalid stable release accepted", version)
			}
		})
	}
}
