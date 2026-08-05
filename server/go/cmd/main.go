package main

import (
	"os"

	"github.com/rss/go-server/backend"
)

func main() {
	srv, err := backend.Start()
	if err != nil {
		os.Exit(1)
	}
	srv.RunBlocking()
}
