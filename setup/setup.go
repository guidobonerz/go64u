package setup

import (
	"drazil.de/go64u/commands"
	"drazil.de/go64u/util"

	"github.com/spf13/cobra"
)

func Setup(cmd *cobra.Command, skipTerminal bool) {
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddGroup(&cobra.Group{ID: "platform", Title: util.YellowText("Platform Commands")})
	cmd.AddGroup(&cobra.Group{ID: "file", Title: util.YellowText("File Commands")})
	cmd.AddGroup(&cobra.Group{ID: "machine", Title: util.YellowText("Machine Commands")})
	cmd.AddGroup(&cobra.Group{ID: "runner", Title: util.YellowText("Runner Commands")})
	cmd.AddGroup(&cobra.Group{ID: "stream", Title: util.YellowText("Stream Commands")})
	cmd.AddGroup(&cobra.Group{ID: "vic", Title: util.YellowText("VIC Commands")})

	cmd.AddCommand(commands.VersionCommand())
	cmd.AddCommand(commands.ActiveDeviceCommand())
	cmd.AddCommand(commands.ShowDevicesCommand())
	cmd.AddCommand(commands.WriteMemoryCommand())
	cmd.AddCommand(commands.ReadMemoryCommand())
	//cmd.AddCommand(commands.DumpPageCommand())
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

}
