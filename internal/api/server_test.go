// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServer_OnlyServesHealth(t *testing.T) {
	handler := NewServer().Handler()
	tests := []struct {
		path string
		want int
	}{
		{"/health", http.StatusOK},
		{"/documents", http.StatusNotFound},
		{"/search?q=x", http.StatusNotFound},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if rec.Code != tt.want {
			t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.want)
		}
	}
}
