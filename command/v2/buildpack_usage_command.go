package v2

import (
	"os"

	"code.cloudfoundry.org/cli/cf/cmd"
	"code.cloudfoundry.org/cli/command"
)

type BuildpackUsageCommand struct {
	usage           interface{} `usage:"CF_NAME buildpack-usage"`
	relatedCommands interface{} `related_commands:"push"`
}

func (BuildpackUsageCommand) Setup(config command.Config, ui command.UI) error {
	return nil
}

func (BuildpackUsageCommand) Execute(args []string) error {
	cmd.Main(os.Getenv("CF_TRACE"), os.Args)
	return nil
}
