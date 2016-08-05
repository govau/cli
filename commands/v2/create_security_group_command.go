package v2

import (
	"os"

	"code.cloudfoundry.org/cli/cf/cmd"
	"code.cloudfoundry.org/cli/commands/flags"
)

type CreateSecurityGroupCommand struct {
	RequiredArgs flags.SecurityGroupArgs `positional-args:"yes"`
}

func (_ CreateSecurityGroupCommand) Execute(args []string) error {
	cmd.Main(os.Getenv("CF_TRACE"), os.Args)
	return nil
}