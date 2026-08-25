package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"path"
)

//go:embed assets/*
var resources embed.FS

type asset struct {
	body        []byte
	contentType string
	etag        string
}

func loadAssets() map[string]asset {
	files, err := fs.Sub(resources, "assets")
	if err != nil {
		panic(err)
	}
	assets := map[string]asset{}
	for _, name := range []string{"index.html", "app.css", "extensions.css", "app.js"} {
		body, readErr := fs.ReadFile(files, name)
		if readErr != nil {
			panic(readErr)
		}
		sum := sha256.Sum256(body)
		contentType := "application/octet-stream"
		switch path.Ext(name) {
		case ".html":
			contentType = "text/html; charset=utf-8"
		case ".css":
			contentType = "text/css; charset=utf-8"
		case ".js":
			contentType = "text/javascript; charset=utf-8"
		}
		assets[name] = asset{body: body, contentType: contentType, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
	}
	return assets
}
