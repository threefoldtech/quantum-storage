package cmd

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"github.com/spf13/cobra"
)

const hookSocketPath = "/tmp/zdb-hook.sock"

// hookCmd represents the hook command
var hookCmd = &cobra.Command{
	Use:   "hook [args...]",
	Short: "A hook to be called by zdb to notify the daemon of events.",
	Long: `This command is called by zdb when certain events occur.
It communicates with the main quantumd daemon via a Unix socket, passing all arguments directly.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Connect to the unix socket
		conn, err := net.Dial("unix", hookSocketPath)
		if err != nil {
			// Log the error to stderr but exit with 0
			fmt.Fprintf(cmd.ErrOrStderr(), "could not connect to quantumd daemon socket at %s: %v. is the daemon running?\n", hookSocketPath, err)
			return nil
		}
		defer conn.Close()

		// Join all arguments into a single space-separated string
		message := strings.Join(args, " ") + "\n"

		// Write the message to the socket
		_, err = conn.Write([]byte(message))
		if err != nil {
			// Log the error to stderr but exit with 0
			fmt.Fprintf(cmd.ErrOrStderr(), "failed to send hook message to daemon: %v\n", err)
			return nil
		}

		// Read response from daemon for blocking hooks
		scanner := bufio.NewScanner(conn)
		if scanner.Scan() {
			response := scanner.Text()
			fmt.Printf("Hook response from daemon: %s\n", response)

			// Exit with appropriate code based on response
			if strings.HasPrefix(response, "ERROR: ") {
				return fmt.Errorf("hook failed: %s", response[7:])
			}
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "error reading response from daemon: %v\n", err)
			return nil
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
