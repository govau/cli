package pushaction_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	. "code.cloudfoundry.org/cli/actor/pushaction"
	"code.cloudfoundry.org/cli/actor/pushaction/manifest"
	"code.cloudfoundry.org/cli/actor/pushaction/pushactionfakes"
	"code.cloudfoundry.org/cli/actor/v2action"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Application Config", func() {
	var (
		actor       *Actor
		fakeV2Actor *pushactionfakes.FakeV2Actor
	)

	BeforeEach(func() {
		fakeV2Actor = new(pushactionfakes.FakeV2Actor)
		actor = NewActor(fakeV2Actor)
	})

	Describe("ApplicationConfig", func() {
		Describe("CreatingApplication", func() {
			Context("when the app did not exist", func() {
				It("returns true", func() {
					config := ApplicationConfig{}
					Expect(config.CreatingApplication()).To(BeTrue())
				})
			})

			Context("when the app exists", func() {
				It("returns false", func() {
					config := ApplicationConfig{CurrentApplication: Application{Application: v2action.Application{GUID: "some-app-guid"}}}
					Expect(config.CreatingApplication()).To(BeFalse())
				})
			})
		})

		Describe("UpdatedApplication", func() {
			Context("when the app did not exist", func() {
				It("returns false", func() {
					config := ApplicationConfig{}
					Expect(config.UpdatingApplication()).To(BeFalse())
				})
			})

			Context("when the app exists", func() {
				It("returns true", func() {
					config := ApplicationConfig{CurrentApplication: Application{Application: v2action.Application{GUID: "some-app-guid"}}}
					Expect(config.UpdatingApplication()).To(BeTrue())
				})
			})
		})
	})

	Describe("ConvertToApplicationConfigs", func() {
		var (
			appName   string
			domain    v2action.Domain
			filesPath string

			orgGUID      string
			spaceGUID    string
			noStart      bool
			manifestApps []manifest.Application

			configs    []ApplicationConfig
			warnings   Warnings
			executeErr error

			firstConfig ApplicationConfig
		)

		BeforeEach(func() {
			appName = "some-app"
			orgGUID = "some-org-guid"
			spaceGUID = "some-space-guid"
			noStart = false

			var err error
			filesPath, err = ioutil.TempDir("", "convert-to-application-configs")
			Expect(err).ToNot(HaveOccurred())

			// The temp directory created on OSX contains a symlink and needs to be evaluated.
			filesPath, err = filepath.EvalSymlinks(filesPath)
			Expect(err).ToNot(HaveOccurred())

			manifestApps = []manifest.Application{{
				Name: appName,
				Path: filesPath,
			}}

			domain = v2action.Domain{
				Name: "private-domain.com",
				GUID: "some-private-domain-guid",
			}
			// Prevents NoDomainsFoundError
			fakeV2Actor.GetOrganizationDomainsReturns(
				[]v2action.Domain{domain},
				v2action.Warnings{"private-domain-warnings", "shared-domain-warnings"},
				nil,
			)
		})

		JustBeforeEach(func() {
			configs, warnings, executeErr = actor.ConvertToApplicationConfigs(orgGUID, spaceGUID, noStart, manifestApps)
			if len(configs) > 0 {
				firstConfig = configs[0]
			}
		})

		AfterEach(func() {
			Expect(os.RemoveAll(filesPath)).ToNot(HaveOccurred())
		})

		Context("when the path is a symlink", func() {
			var target string

			BeforeEach(func() {
				parentDir := filepath.Dir(filesPath)
				target = filepath.Join(parentDir, "i-r-symlink")
				Expect(os.Symlink(filesPath, target)).ToNot(HaveOccurred())
				manifestApps[0].Path = target
			})

			AfterEach(func() {
				Expect(os.RemoveAll(target)).ToNot(HaveOccurred())
			})

			It("evaluates the symlink into an absolute path", func() {
				Expect(executeErr).ToNot(HaveOccurred())

				Expect(firstConfig.Path).To(Equal(filesPath))
			})

			Context("given a path that does not exist", func() {
				BeforeEach(func() {
					manifestApps[0].Path = "/i/will/fight/you/if/this/exists"
				})

				It("returns errors and warnings", func() {
					Expect(os.IsNotExist(executeErr)).To(BeTrue())

					Expect(fakeV2Actor.GatherDirectoryResourcesCallCount()).To(Equal(0))
					Expect(fakeV2Actor.GatherArchiveResourcesCallCount()).To(Equal(0))
				})
			})
		})

		Context("when the application exists", func() {
			var app Application
			var route v2action.Route

			BeforeEach(func() {
				app = Application{
					Application: v2action.Application{
						Name:      appName,
						GUID:      "some-app-guid",
						SpaceGUID: spaceGUID,
					}}

				route = v2action.Route{
					Domain: v2action.Domain{
						Name: "some-domain.com",
						GUID: "some-domain-guid",
					},
					Host:      app.Name,
					GUID:      "route-guid",
					SpaceGUID: spaceGUID,
				}

				fakeV2Actor.GetApplicationByNameAndSpaceReturns(app.Application, v2action.Warnings{"some-app-warning-1", "some-app-warning-2"}, nil)
			})

			Context("when retrieving the application's routes is successful", func() {
				BeforeEach(func() {
					fakeV2Actor.GetApplicationRoutesReturns([]v2action.Route{route}, v2action.Warnings{"app-route-warnings"}, nil)
				})

				Context("when retrieving the application's services is successful", func() {
					var services []v2action.ServiceInstance

					BeforeEach(func() {
						services = []v2action.ServiceInstance{
							{Name: "service-1"},
							{Name: "service-2"},
						}

						fakeV2Actor.GetServiceInstancesByApplicationReturns(services, v2action.Warnings{"service-instance-warning-1", "service-instance-warning-2"}, nil)
					})

					It("return warnings", func() {
						Expect(executeErr).ToNot(HaveOccurred())
						Expect(warnings).To(ConsistOf("some-app-warning-1", "some-app-warning-2", "app-route-warnings", "private-domain-warnings", "shared-domain-warnings", "service-instance-warning-1", "service-instance-warning-2"))
					})

					It("sets the current application to the existing application", func() {
						Expect(firstConfig.CurrentApplication).To(Equal(app))
						Expect(firstConfig.TargetedSpaceGUID).To(Equal(spaceGUID))

						Expect(fakeV2Actor.GetApplicationByNameAndSpaceCallCount()).To(Equal(1))
						passedName, passedSpaceGUID := fakeV2Actor.GetApplicationByNameAndSpaceArgsForCall(0)
						Expect(passedName).To(Equal(app.Name))
						Expect(passedSpaceGUID).To(Equal(spaceGUID))
					})

					It("sets the current routes", func() {
						Expect(firstConfig.CurrentRoutes).To(ConsistOf(route))

						Expect(fakeV2Actor.GetApplicationRoutesCallCount()).To(Equal(1))
						Expect(fakeV2Actor.GetApplicationRoutesArgsForCall(0)).To(Equal(app.GUID))
					})

					It("sets the bound services", func() {
						Expect(firstConfig.CurrentServices).To(Equal(services))

						Expect(fakeV2Actor.GetServiceInstancesByApplicationCallCount()).To(Equal(1))
						Expect(fakeV2Actor.GetServiceInstancesByApplicationArgsForCall(0)).To(Equal("some-app-guid"))
					})
				})

				Context("when retrieving the application's services errors", func() {
					var expectedErr error

					BeforeEach(func() {
						expectedErr = errors.New("dios mio")
						fakeV2Actor.GetServiceInstancesByApplicationReturns(nil, v2action.Warnings{"service-instance-warning-1", "service-instance-warning-2"}, expectedErr)
					})

					It("returns the error and warnings", func() {
						Expect(executeErr).To(MatchError(expectedErr))
						Expect(warnings).To(ConsistOf("some-app-warning-1", "some-app-warning-2", "app-route-warnings", "service-instance-warning-1", "service-instance-warning-2"))
					})
				})
			})

			Context("when retrieving the application's routes errors", func() {
				var expectedErr error

				BeforeEach(func() {
					expectedErr = errors.New("dios mio")
					fakeV2Actor.GetApplicationRoutesReturns(nil, v2action.Warnings{"app-route-warnings"}, expectedErr)
				})

				It("returns the error and warnings", func() {
					Expect(executeErr).To(MatchError(expectedErr))
					Expect(warnings).To(ConsistOf("some-app-warning-1", "some-app-warning-2", "app-route-warnings"))
				})
			})
		})

		Context("when the application does not exist", func() {
			BeforeEach(func() {
				fakeV2Actor.GetApplicationByNameAndSpaceReturns(v2action.Application{}, v2action.Warnings{"some-app-warning-1", "some-app-warning-2"}, v2action.ApplicationNotFoundError{})
			})

			It("creates a new application and sets it to the desired application", func() {
				Expect(executeErr).ToNot(HaveOccurred())
				Expect(warnings).To(ConsistOf("some-app-warning-1", "some-app-warning-2", "private-domain-warnings", "shared-domain-warnings"))
				Expect(firstConfig.CurrentApplication).To(Equal(Application{Application: v2action.Application{}}))
				Expect(firstConfig.DesiredApplication).To(Equal(Application{
					Application: v2action.Application{
						Name:      "some-app",
						SpaceGUID: spaceGUID,
					}}))
				Expect(firstConfig.TargetedSpaceGUID).To(Equal(spaceGUID))
			})
		})

		Context("when retrieving the application errors", func() {
			var expectedErr error

			BeforeEach(func() {
				expectedErr = errors.New("dios mio")
				fakeV2Actor.GetApplicationByNameAndSpaceReturns(v2action.Application{}, v2action.Warnings{"some-app-warning-1", "some-app-warning-2"}, expectedErr)
			})

			It("returns the error and warnings", func() {
				Expect(executeErr).To(MatchError(expectedErr))
				Expect(warnings).To(ConsistOf("some-app-warning-1", "some-app-warning-2"))
			})
		})

		Context("when overriding application properties", func() {
			var stack v2action.Stack

			Context("when the manifest contains all the properties", func() {
				BeforeEach(func() {
					manifestApps[0].BuildpackName = "some-buildpack"
					manifestApps[0].Command = "some-buildpack"
					manifestApps[0].HealthCheckHTTPEndpoint = "some-buildpack"
					manifestApps[0].HealthCheckTimeout = 5
					manifestApps[0].HealthCheckType = "some-buildpack"
					manifestApps[0].Instances = 1
					manifestApps[0].DiskQuota = 2
					manifestApps[0].Memory = 3
					manifestApps[0].StackName = "some-stack"
					manifestApps[0].EnvironmentVariables = map[string]string{
						"env1": "1",
						"env3": "3",
					}

					stack = v2action.Stack{
						Name: "some-stack",
						GUID: "some-stack-guid",
					}

					fakeV2Actor.GetStackByNameReturns(stack, v2action.Warnings{"some-stack-warning"}, nil)
				})

				It("overrides the current application properties", func() {
					Expect(warnings).To(ConsistOf("some-stack-warning", "private-domain-warnings", "shared-domain-warnings"))

					Expect(firstConfig.DesiredApplication.Buildpack).To(Equal("some-buildpack"))
					Expect(firstConfig.DesiredApplication.Command).To(Equal("some-buildpack"))
					Expect(firstConfig.DesiredApplication.EnvironmentVariables).To(Equal(map[string]string{
						"env1": "1",
						"env3": "3",
					}))
					Expect(firstConfig.DesiredApplication.HealthCheckHTTPEndpoint).To(Equal("some-buildpack"))
					Expect(firstConfig.DesiredApplication.HealthCheckTimeout).To(Equal(5))
					Expect(firstConfig.DesiredApplication.HealthCheckType).To(Equal("some-buildpack"))
					Expect(firstConfig.DesiredApplication.Instances).To(Equal(1))
					Expect(firstConfig.DesiredApplication.DiskQuota).To(BeNumerically("==", 2))
					Expect(firstConfig.DesiredApplication.Memory).To(BeNumerically("==", 3))
					Expect(firstConfig.DesiredApplication.StackGUID).To(Equal("some-stack-guid"))
					Expect(firstConfig.DesiredApplication.Stack).To(Equal(stack))

					Expect(fakeV2Actor.GetStackByNameCallCount()).To(Equal(1))
					Expect(fakeV2Actor.GetStackByNameArgsForCall(0)).To(Equal("some-stack"))
				})
			})

			Context("when the manifest does not contain any properties", func() {
				BeforeEach(func() {
					stack = v2action.Stack{
						Name: "some-stack",
						GUID: "some-stack-guid",
					}
					fakeV2Actor.GetStackReturns(stack, nil, nil)

					app := v2action.Application{
						Buildpack: "some-buildpack",
						Command:   "some-buildpack",
						DiskQuota: 2,
						EnvironmentVariables: map[string]string{
							"env2": "2",
							"env3": "9",
						},
						GUID: "some-app-guid",
						HealthCheckHTTPEndpoint: "some-buildpack",
						HealthCheckTimeout:      5,
						HealthCheckType:         "some-buildpack",
						Instances:               1,
						Memory:                  3,
						Name:                    appName,
						StackGUID:               stack.GUID,
					}
					fakeV2Actor.GetApplicationByNameAndSpaceReturns(app, nil, nil)
				})

				It("keeps the original app properties", func() {
					Expect(firstConfig.DesiredApplication.Buildpack).To(Equal("some-buildpack"))
					Expect(firstConfig.DesiredApplication.Command).To(Equal("some-buildpack"))
					Expect(firstConfig.DesiredApplication.EnvironmentVariables).To(Equal(map[string]string{
						"env2": "2",
						"env3": "9",
					}))
					Expect(firstConfig.DesiredApplication.HealthCheckHTTPEndpoint).To(Equal("some-buildpack"))
					Expect(firstConfig.DesiredApplication.HealthCheckTimeout).To(Equal(5))
					Expect(firstConfig.DesiredApplication.HealthCheckType).To(Equal("some-buildpack"))
					Expect(firstConfig.DesiredApplication.Instances).To(Equal(1))
					Expect(firstConfig.DesiredApplication.DiskQuota).To(BeNumerically("==", 2))
					Expect(firstConfig.DesiredApplication.Memory).To(BeNumerically("==", 3))
					Expect(firstConfig.DesiredApplication.StackGUID).To(Equal("some-stack-guid"))
					Expect(firstConfig.DesiredApplication.Stack).To(Equal(stack))
				})
			})

			Context("when retrieving the stack errors", func() {
				var expectedErr error

				BeforeEach(func() {
					manifestApps[0].StackName = "some-stack"

					expectedErr = errors.New("potattototototototootot")
					fakeV2Actor.GetStackByNameReturns(v2action.Stack{}, v2action.Warnings{"some-stack-warning"}, expectedErr)
				})

				It("returns the error and warnings", func() {
					Expect(executeErr).To(MatchError(expectedErr))
					Expect(warnings).To(ConsistOf("some-stack-warning"))
				})
			})

			Context("when both the manifest and application contains environment variables", func() {
				BeforeEach(func() {
					manifestApps[0].EnvironmentVariables = map[string]string{
						"env1": "1",
						"env3": "3",
					}

					app := v2action.Application{
						EnvironmentVariables: map[string]string{
							"env2": "2",
							"env3": "9",
						},
					}
					fakeV2Actor.GetApplicationByNameAndSpaceReturns(app, nil, nil)
				})

				It("adds/overrides the existing environment variables", func() {
					Expect(firstConfig.DesiredApplication.EnvironmentVariables).To(Equal(map[string]string{
						"env1": "1",
						"env2": "2",
						"env3": "3",
					}))

					// Does not modify original set of env vars
					Expect(firstConfig.CurrentApplication.EnvironmentVariables).To(Equal(map[string]string{
						"env2": "2",
						"env3": "9",
					}))
				})
			})

			Context("when neither the manifest or the application contains environment variables", func() {
				It("leaves the EnvironmentVariables as nil", func() {
					Expect(firstConfig.DesiredApplication.EnvironmentVariables).To(BeNil())
				})
			})

			Context("when no-start is set to true", func() {
				BeforeEach(func() {
					noStart = true
				})

				It("sets the desired app state to stopped", func() {
					Expect(executeErr).ToNot(HaveOccurred())
					Expect(firstConfig.DesiredApplication.Stopped()).To(BeTrue())
				})
			})
		})

		Context("when retrieving the default route is successful", func() {
			BeforeEach(func() {
				// Assumes new route
				fakeV2Actor.FindRouteBoundToSpaceWithSettingsReturns(v2action.Route{}, v2action.Warnings{"get-route-warnings"}, v2action.RouteNotFoundError{})
			})

			It("adds the route to desired routes", func() {
				Expect(executeErr).ToNot(HaveOccurred())
				Expect(warnings).To(ConsistOf("private-domain-warnings", "shared-domain-warnings", "get-route-warnings"))
				Expect(firstConfig.DesiredRoutes).To(ConsistOf(v2action.Route{
					Domain:    domain,
					Host:      appName,
					SpaceGUID: spaceGUID,
				}))
			})
		})

		Context("when retrieving the default route errors", func() {
			var expectedErr error

			BeforeEach(func() {
				expectedErr = errors.New("dios mio")
				fakeV2Actor.FindRouteBoundToSpaceWithSettingsReturns(v2action.Route{}, v2action.Warnings{"get-route-warnings"}, expectedErr)
			})

			It("returns the error and warnings", func() {
				Expect(executeErr).To(MatchError(expectedErr))
				Expect(warnings).To(ConsistOf("private-domain-warnings", "shared-domain-warnings", "get-route-warnings"))
			})
		})

		Context("when scanning for files", func() {
			Context("given a directory", func() {
				Context("when scanning is successful", func() {
					var resources []v2action.Resource

					BeforeEach(func() {
						resources = []v2action.Resource{
							{Filename: "I am a file!"},
							{Filename: "I am not a file"},
						}
						fakeV2Actor.GatherDirectoryResourcesReturns(resources, nil)
					})

					It("sets the full resource list on the config", func() {
						Expect(executeErr).ToNot(HaveOccurred())
						Expect(warnings).To(ConsistOf("private-domain-warnings", "shared-domain-warnings"))
						Expect(firstConfig.AllResources).To(Equal(resources))
						Expect(firstConfig.Path).To(Equal(filesPath))
						Expect(firstConfig.Archive).To(BeFalse())

						Expect(fakeV2Actor.GatherDirectoryResourcesCallCount()).To(Equal(1))
						Expect(fakeV2Actor.GatherDirectoryResourcesArgsForCall(0)).To(Equal(filesPath))
					})
				})

				Context("when scanning errors", func() {
					var expectedErr error

					BeforeEach(func() {
						expectedErr = errors.New("dios mio")
						fakeV2Actor.GatherDirectoryResourcesReturns(nil, expectedErr)
					})

					It("returns the error and warnings", func() {
						Expect(executeErr).To(MatchError(expectedErr))
						Expect(warnings).To(ConsistOf("private-domain-warnings", "shared-domain-warnings"))
					})
				})
			})

			Context("given an archive", func() {
				var archive string

				BeforeEach(func() {
					f, err := ioutil.TempFile("", "convert-to-application-configs-archive")
					Expect(err).ToNot(HaveOccurred())
					archive = f.Name()
					Expect(f.Close()).ToNot(HaveOccurred())

					// The temp file created on OSX contains a symlink and needs to be evaluated.
					archive, err = filepath.EvalSymlinks(archive)
					Expect(err).ToNot(HaveOccurred())

					manifestApps[0].Path = archive
				})

				AfterEach(func() {
					Expect(os.RemoveAll(archive)).ToNot(HaveOccurred())
				})

				Context("when scanning is successful", func() {
					var resources []v2action.Resource

					BeforeEach(func() {
						resources = []v2action.Resource{
							{Filename: "I am a file!"},
							{Filename: "I am not a file"},
						}
						fakeV2Actor.GatherArchiveResourcesReturns(resources, nil)
					})

					It("sets the full resource list on the config", func() {
						Expect(executeErr).ToNot(HaveOccurred())
						Expect(warnings).To(ConsistOf("private-domain-warnings", "shared-domain-warnings"))
						Expect(firstConfig.AllResources).To(Equal(resources))
						Expect(firstConfig.Path).To(Equal(archive))
						Expect(firstConfig.Archive).To(BeTrue())

						Expect(fakeV2Actor.GatherArchiveResourcesCallCount()).To(Equal(1))
						Expect(fakeV2Actor.GatherArchiveResourcesArgsForCall(0)).To(Equal(archive))
					})
				})

				Context("when scanning errors", func() {
					var expectedErr error

					BeforeEach(func() {
						expectedErr = errors.New("dios mio")
						fakeV2Actor.GatherArchiveResourcesReturns(nil, expectedErr)
					})

					It("returns the error and warnings", func() {
						Expect(executeErr).To(MatchError(expectedErr))
						Expect(warnings).To(ConsistOf("private-domain-warnings", "shared-domain-warnings"))
					})
				})
			})
		})

		Context("when a docker image is configured", func() {
			BeforeEach(func() {
				manifestApps[0].DockerImage = "some-docker-image-path"
			})

			It("sets the docker image on DesiredApplication and does not gather resources", func() {
				Expect(executeErr).ToNot(HaveOccurred())
				Expect(firstConfig.DesiredApplication.DockerImage).To(Equal("some-docker-image-path"))

				Expect(fakeV2Actor.GatherDirectoryResourcesCallCount()).To(Equal(0))
			})
		})
	})
})
