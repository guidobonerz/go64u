package setup

import (
	"de/drazil/go64u/commands"
	"de/drazil/go64u/util"

	"github.com/spf13/cobra"
)

func Setup(cmd *cobra.Command, skipTerminal bool) {
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddGroup(&cobra.Group{ID: "platform", Title: util.YellowText("Platform related Commands")})
	cmd.AddGroup(&cobra.Group{ID: "file", Title: util.YellowText("File related Commands")})
	cmd.AddGroup(&cobra.Group{ID: "machine", Title: util.YellowText("Machine related Commands")})
	cmd.AddGroup(&cobra.Group{ID: "runner", Title: util.YellowText("Runner related Commands")})
	cmd.AddGroup(&cobra.Group{ID: "stream", Title: util.YellowText("Stream related Commands")})
	cmd.AddGroup(&cobra.Group{ID: "vic", Title: util.YellowText("VIC related Commands")})

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
