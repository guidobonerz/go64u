package commands

import (
	"log"

	"github.com/spf13/cobra"
)

func VideoStream() *cobra.Command {
	return &cobra.Command{
		Use:     "video [command]",
		Short:   "Starts/Stops the video stream",
		Long:    "Starts/Stops the video stream",
		GroupID: "stream",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("Not yet implemented!!")
		},
	}
}

func AudioStream() *cobra.Command {
	return &cobra.Command{
		Use:     "audio [command]",
		Short:   "Starts/Stops the audio stream",
		Long:    "Starts/Stops the audio stream",
		GroupID: "stream",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("Not yet implemented!!")
		},
	}
}

func DebugStream() *cobra.Command {
	return &cobra.Command{
		Use:     "debug [command]",
		Short:   "Starts/Stops the debug stream",
		Long:    "Starts/Stops the debug stream, audio and video streams will be then stopped",
		GroupID: "stream",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("Not yet implemented!!")
		},
	}
}

func Screenshot() *cobra.Command {
	return &cobra.Command{
		Use:     "screenshot [format]",
		Short:   "Makes a screen of the current screen",
		Long:    "Makes a screen of the current screen",
		GroupID: "stream",
		Args:    cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("Not yet implemented!!")
		},
	}
}
