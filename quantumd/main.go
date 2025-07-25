package main

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/threefoldtech/quantum-storage/quantumd/cmd"
)

//go:embed all:assets
var assets embed.FS

func main() {
	// Get the base name of the command used to execute the program
	exeName := filepath.Base(os.Args[0])

	// If the binary is called via a name starting with "quantumd-hook"
	// (e.g. a symlink), then we insert "hook" as the first argument.
	// This makes `quantumd-hook arg1 arg2` behave like `quantumd hook arg1 arg2`.
	if strings.HasPrefix(exeName, "quantumd-hook") {
		// os.Args = ["quantumd-hook", "arg1", "arg2"]
		// becomes:
		// os.Args = ["quantumd-hook", "hook", "arg1", "arg2"]
		os.Args = append([]string{os.Args[0], "hook"}, os.Args[1:]...)
	}

	cmd.SetAssets(assets)
	cmd.Execute()
}
