package main

import (
	"embed"
	"os"

	"github.com/threefoldtech/quantum-daemon/cmd"
)

//go:embed assets/systemd/* assets/zinit/*
var serviceFiles embed.FS

func main() {
	cmd.ServiceFiles = serviceFiles
	if len(os.Args) > 1 && os.Args[1] == "local" {
		cmd.LocalMode = true
	}
	cmd.Execute()
}
