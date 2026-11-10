package distribution_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CAUCraft/LXMCL/internal/distribution"
)

func TestExportReportsCompletionOnlyForVerifiedFiles(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprintf("valid=%t", valid), func(t *testing.T) {
			const body = "verified resource file"
			dir := t.TempDir()
			name := filepath.Join(dir, "file.zip")
			c := distribution.Cache{Root: t.TempDir(), Client: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
				data := body
				if !valid {
					data = strings.Repeat("x", len(body))
				}
				return &http.Response{StatusCode: 200, ContentLength: int64(len(data)), Body: io.NopCloser(strings.NewReader(data))}, nil
			})}}
			completed := false
			err := c.Export(t.Context(), asset(body), "file.zip", dir, func(done, total int) {
				if done == 0 {
					return
				}
				completed = true
				data, err := os.ReadFile(name)
				if done != 1 || total != 1 || err != nil || string(data) != body {
					t.Fatalf("completed before verified publication: %d/%d, %q, %v", done, total, data, err)
				}
			})
			if (err == nil) != valid || completed != valid {
				t.Fatalf("export: %v, completed=%t", err, completed)
			}
			wantFiles := 0
			if valid {
				wantFiles = 1
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != wantFiles {
				t.Fatalf("unexpected files: %v, %v", entries, err)
			}
		})
	}
}
