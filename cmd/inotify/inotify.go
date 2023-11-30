package inotify

import (
	"context"

	"github.com/rclone/rclone/cmd"
	"github.com/rclone/rclone/fs/config/flags"
	"github.com/rclone/rclone/fs/inotify"
	"github.com/spf13/cobra"
)

var (
	createEmptySrcDirs = false
)

func init() {
	cmd.Root.AddCommand(commandDefinition)
	cmdFlags := commandDefinition.Flags()
	flags.BoolVarP(cmdFlags, &createEmptySrcDirs, "create-empty-src-dirs", "", createEmptySrcDirs, "Create empty source dirs on destination after sync", "")
}

var commandDefinition = &cobra.Command{
	Use:   "inotify source:path dest:path",
	Short: `Inotify source path and sync source:path to dest:path`,
	Annotations: map[string]string{
		"groups": "Sync,Copy,Filter,Listing,Important",
	},
	Run: func(command *cobra.Command, args []string) {
		cmd.CheckArgs(2, 2, command, args)
		fsrc, srcFileName, fdst := cmd.NewFsSrcFileDst(args)
		cmd.Run(false, false, command, func() error {
			return inotify.Inotify(context.Background(), fdst, fsrc, srcFileName, createEmptySrcDirs)
		})
	},
}
