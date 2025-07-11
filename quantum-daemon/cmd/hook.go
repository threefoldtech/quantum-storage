package cmd

import (
	"fmt"
	"net"
	"strings"

	"github.com/spf13/cobra"
)

const socketPath = "/tmp/quantumd.sock"

// hookCmd represents the hook command
var hookCmd = &cobra.Command{
	Use:   "hook [args...]",
	Short: "A hook to be called by zdb to notify the daemon of events.",
	Long: `This command is called by zdb when certain events occur.
It communicates with the main quantumd daemon via a Unix socket, passing all arguments directly.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Connect to the unix socket
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			return fmt.Errorf("could not connect to quantumd daemon socket at %s: %w. is the daemon running?", socketPath, err)
		}
		defer conn.Close()

		// Join all arguments into a single space-separated string
		message := strings.Join(args, " ") + "\n"

		// Write the message to the socket
		_, err = conn.Write([]byte(message))
		if err != nil {
			return fmt.Errorf("failed to send hook message to daemon: %w", err)
		}

		fmt.Printf("Hook message sent to daemon: %s", message)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
