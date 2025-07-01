package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AshkanYarmoradi/hcledit/cmd/hcledit/internal/version"
)

func NewCmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the version and revision",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s (%s)\n", version.Version, version.Revision)
		},
	}
}
