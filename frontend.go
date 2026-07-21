package k2board

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/dist
var distFS embed.FS

// FrontendFS returns the embedded Vue frontend filesystem.
// Returns nil if the frontend was not built (e.g., during development).
func FrontendFS() http.FileSystem {
	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		return nil
	}
	return http.FS(sub)
}
