package frontend

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var frontendFS embed.FS

func GetFS() (fs.FS, error) {
	sub, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return nil, err
	}
	return sub, nil
}