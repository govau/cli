package reports

import (
	"strings"

	"code.cloudfoundry.org/cli/cf/api/applications"
	"code.cloudfoundry.org/cli/cf/commandregistry"
	"code.cloudfoundry.org/cli/cf/flags"
	. "code.cloudfoundry.org/cli/cf/i18n"
	"code.cloudfoundry.org/cli/cf/models"

	"code.cloudfoundry.org/cli/cf/configuration/coreconfig"
	"code.cloudfoundry.org/cli/cf/requirements"
	"code.cloudfoundry.org/cli/cf/terminal"
)

type ListBuildpackUsage struct {
	ui      terminal.UI
	config  coreconfig.Reader
	appRepo applications.Repository

	pluginCall bool
}

func init() {
	commandregistry.Register(&ListBuildpackUsage{})
}

func (cmd *ListBuildpackUsage) MetaData() commandregistry.CommandMetadata {
	return commandregistry.CommandMetadata{
		Name:        "buildpack-usage",
		ShortName:   "bpu",
		Description: T("List all buildpacks and the apps that use them"),
		Usage: []string{
			"CF_NAME buildpack-usage",
		},
	}
}

func (cmd *ListBuildpackUsage) Requirements(requirementsFactory requirements.Factory, fc flags.FlagContext) ([]requirements.Requirement, error) {
	usageReq := requirements.NewUsageRequirement(commandregistry.CLICommandUsagePresenter(cmd),
		T("No argument required"),
		func() bool {
			return len(fc.Args()) != 0
		},
	)

	reqs := []requirements.Requirement{
		usageReq,
		requirementsFactory.NewLoginRequirement(),
	}

	return reqs, nil
}

func (cmd *ListBuildpackUsage) SetDependency(deps commandregistry.Dependency, pluginCall bool) commandregistry.Command {
	cmd.ui = deps.UI
	cmd.config = deps.Config
	cmd.appRepo = deps.RepoLocator.GetApplicationRepository()
	cmd.pluginCall = pluginCall
	return cmd
}

func (cmd *ListBuildpackUsage) Execute(c flags.FlagContext) error {
	cmd.ui.Say(T("Getting apps from all orgs / spaces as {{.Username}}...",
		map[string]interface{}{
			"Username": terminal.EntityNameColor(cmd.config.Username())}))

	apps, err := cmd.appRepo.ListAllApps()
	if err != nil {
		return err
	}

	cmd.ui.Ok()
	cmd.ui.Say("")

	if len(apps) == 0 {
		cmd.ui.Say(T("No apps found"))
		return nil
	}

	buildpackToApps := make(map[string][]models.Application)
	for _, app := range apps {
		buildpackToApps[app.Buildpack] = append(buildpackToApps[app.Buildpack], app)
	}

	table := cmd.ui.Table([]string{
		T("buildpack"),
		T("applications"),
	})
	for bp, bpApps := range buildpackToApps {
		var lines []string
		for _, app := range bpApps {
			if bp == "" {
				lines = append(lines, app.Name+" (detected: "+app.DetectedBuildpack+")")
			} else {
				lines = append(lines, app.Name)
			}
		}

		if bp == "" {
			bp = "(blank)"
		}

		table.Add(
			bp,
			strings.Join(lines, "\n")+"\n",
		)
	}

	err = table.Print()
	if err != nil {
		return err
	}
	return nil
}
