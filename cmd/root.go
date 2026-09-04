// Package cmd defines the bililive-cli command tree.
package cmd

import (
	"context"
	"os"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"
)

// version is set at build time with -ldflags.
var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "bililive",
	Short: "Follow bilibili live streams from your terminal",
	Long:  "bililive follows bilibili live rooms and logs every audience interaction. You can also send comments through it",
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}

// Execute runs the root command. Fang renders help, --version, man pages,
// and errors. It exits non-zero on error.
func Execute() {
	if err := fang.Execute(context.Background(), rootCmd, fang.WithVersion(version)); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(whoamiCmd)
	rootCmd.AddCommand(sendCmd)
}
