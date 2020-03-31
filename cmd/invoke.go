package cmd

import (
	"github.com/spf13/cobra"
)

var invokeCmd = &cobra.Command{
	Use:   "invoke",
	Short: "Invoke a specified command for live testing",
}

func init() {
	rootCmd.AddCommand(invokeCmd)
}
