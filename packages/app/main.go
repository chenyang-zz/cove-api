package main

import (
	"embed"
	"log"

	coveapp "github.com/chenyang-zz/cove/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := coveapp.New(assets).Run(); err != nil {
		log.Fatal(err)
	}
}
