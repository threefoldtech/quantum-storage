package main

import (
	"embed"

	"github.com/threefoldtech/quantum-daemon/cmd"
)

//go:embed assets/systemd/* assets/zinit/*
var serviceFiles embed.FS

func main() {
	cmd.ServiceFiles = serviceFiles
	cmd.Execute()
}
