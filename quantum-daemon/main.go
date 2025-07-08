package main

import (
	"os"

	"github.com/threefoldtech/quantum-daemon/cmd"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "local" {
		cmd.LocalMode = true
	}
	cmd.Execute()
}
