package setup

import (
	"de/drazil/go64u/commands"

	"github.com/spf13/cobra"
)

func Setup(cmd *cobra.Command, skipTerminal bool) {
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddGroup(&cobra.Group{ID: "platform", Title: "Platform related Commands"})
	cmd.AddGroup(&cobra.Group{ID: "file", Title: "File related Commands"})
	cmd.AddGroup(&cobra.Group{ID: "machine", Title: "Machine related Commands"})
	cmd.AddGroup(&cobra.Group{ID: "runner", Title: "Runner related Commands"})
	cmd.AddGroup(&cobra.Group{ID: "stream", Title: "Stream related Commands"})
	cmd.AddGroup(&cobra.Group{ID: "vic", Title: "VIC related Commands"})

	cmd.AddCommand(commands.VersionCommand())
	cmd.AddCommand(commands.WriteMemoryCommand())
	cmd.AddCommand(commands.ReadMemoryCommand())
	cmd.AddCommand(commands.DumpPageCommand())
	cmd.AddCommand(commands.MessageCommand())
	cmd.AddCommand(commands.PauseCommand())
	cmd.AddCommand(commands.PowerOffCommand())
	cmd.AddCommand(commands.RebootCommand())
	cmd.AddCommand(commands.ResetCommand())
	cmd.AddCommand(commands.ResumeCommand())
	cmd.AddCommand(commands.ToggleMenuCommand())
	cmd.AddCommand(commands.LoadCommand())
	cmd.AddCommand(commands.RunCommand())
	cmd.AddCommand(commands.CrtCommand())
	cmd.AddCommand(commands.VideoStreamCommand())
	cmd.AddCommand(commands.AudioStreamCommand())
	cmd.AddCommand(commands.DebugStreamCommand())
	cmd.AddCommand(commands.ScreenshotCommand())
	cmd.AddCommand(commands.MountCommand())
	cmd.AddCommand(commands.UnmountCommand())
	cmd.AddCommand(commands.DeviceInfoCommand())
	cmd.AddCommand(commands.ScreenControlCommand())
	cmd.AddCommand(commands.RemoteLsCommand())
	cmd.AddCommand(commands.RemoteCdCommand())

}
