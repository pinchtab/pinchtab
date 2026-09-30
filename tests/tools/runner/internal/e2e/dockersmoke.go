package e2e

import (
	"fmt"
	"strings"
	"time"
)

type dockerSmokeStep struct {
	Name                 string
	Tags                 []string
	Command              []string
	ProvidesReleaseImage bool
	ProvidesChromeImage  bool
	RequiresReleaseImage bool
	RequiresChromeImage  bool
}

func (r *Runner) runSmokeFiltered(filter string) int {
	if r.args.Filter == "" {
		r.args.Filter = filter
	}
	return r.runSmokeLane(smokeLane())
}

func (r *Runner) runSmokeLane(l lane) int {
	plans, code := r.planSuites(l.defs)
	if code != 0 {
		return code
	}
	docker := r.selectedDockerSmokePlan()
	dockerSteps := docker.steps
	if len(plans) == 0 && len(dockerSteps) == 0 {
		return r.reportNoSuitesMatched(l)
	}

	codes := map[string]int{}
	otherFailure := false
	if len(plans) > 0 {
		planCodes, restartFailed, setupCode := r.bringUpAndRunPlans(l.stack, plans)
		if setupCode != 0 {
			_ = r.composeDown(l.stack)
			return setupCode
		}
		codes = planCodes
		otherFailure = restartFailed
		if code := r.composeDown(l.stack); code != 0 {
			otherFailure = true
		}
	}

	if len(dockerSteps) > 0 {
		if code := r.runDockerSmokeSteps(docker); code != 0 {
			codes[dockerSmokeSuite().Name] = code
		}
		_, _ = fmt.Fprintln(r.stdout, "")
	}

	return r.reportLaneOutcome(l, codes, otherFailure)
}

func (r *Runner) runDockerSmokeSteps(plan dockerSmokePlan) int {
	def := dockerSmokeSuite()
	r.printSuiteStart(def)
	for _, image := range plan.images {
		_, _ = fmt.Fprintf(r.stdout, "  image %-32s %s\n", image.Ref(), image.Reason)
	}
	_, _ = fmt.Fprintln(r.stdout, "")
	r.prepareSuiteResults(def)
	started := time.Now()

	exitCode := 0
	for _, step := range plan.steps {
		stepStarted := time.Now()
		code := r.runLoggedCommand("running "+step.Name, def.Output, step.Command)
		status := "passed"
		if code != 0 {
			status = "failed"
			exitCode = code
		}
		if err := r.appendSuiteResult(def, status, time.Since(stepStarted), "["+def.Name+"] "+step.Name); err != nil {
			_, _ = fmt.Fprintf(r.stderr, "e2e: failed to record docker smoke result: %v\n", err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
		if code != 0 {
			break
		}
	}

	duration := time.Since(started)
	summary := r.writeSuiteReports(def, duration, exitCode)
	r.recordOverallSummary(summary)
	r.printSuiteSummary(def, summary, duration)
	if exitCode != 0 {
		r.showFailureArtifacts(def, duration)
	}
	return exitCode
}

// dockerSmokePlan is the docker smoke lane's steps together with the image decisions that
// produced them, so the run can state per image whether it built or reused and why.
type dockerSmokePlan struct {
	steps  []dockerSmokeStep
	images []smokeImage
}

var (
	releaseSmokeImageSpec = smokeImageSpec{
		Repo:       "pinchtab-release-smoke",
		Dockerfile: "Dockerfile",
		EnvVar:     "PINCHTAB_DOCKER_SMOKE_RELEASE_IMAGE",
	}
	chromeSmokeImageSpec = smokeImageSpec{
		Repo:       "pinchtab-chrome-cft-smoke",
		Dockerfile: "tests/tools/docker/chrome-cft-smoke.Dockerfile",
		Platform:   "linux/amd64",
		EnvVar:     "PINCHTAB_DOCKER_SMOKE_CHROME_IMAGE",
	}
)

func (r *Runner) selectedDockerSmokePlan() dockerSmokePlan {
	plan := r.dockerSmokePlan()
	plan.steps = selectDockerSmokeSteps(plan.steps, r.args.Filter)
	return plan
}

func (r *Runner) dockerSmokePlan() dockerSmokePlan {
	release := r.resolveSmokeImage(releaseSmokeImageSpec, r.localImageExists)
	chrome := r.resolveSmokeImage(chromeSmokeImageSpec, r.localImageExists)
	releaseImage, chromeImage := release.Ref(), chrome.Ref()

	steps := []dockerSmokeStep{}
	if release.Build {
		steps = append(steps, dockerSmokeStep{
			Name:                 "docker: build release image",
			Tags:                 []string{"docker", "build", "release", "image"},
			Command:              []string{"docker", "build", "--load", "-t", releaseImage, "."},
			ProvidesReleaseImage: true,
		})
	}
	if chrome.Build {
		steps = append(steps, dockerSmokeStep{
			Name:                "docker: build Chrome for Testing image",
			Tags:                []string{"docker", "build", "chrome", "cft", "image"},
			Command:             []string{"docker", "build", "--load", "--platform", chromeSmokeImageSpec.Platform, "-f", chromeSmokeImageSpec.Dockerfile, "-t", chromeImage, "."},
			ProvidesChromeImage: true,
		})
	}
	steps = append(steps,
		dockerSmokeStep{
			Name:                 "docker: bootstrap path in container",
			Tags:                 []string{"docker", "bootstrap", "release", "container", "config"},
			Command:              []string{"bash", "scripts/docker-smoke.sh", releaseImage},
			RequiresReleaseImage: true,
		},
		dockerSmokeStep{
			Name:                "docker: Chrome for Testing startup",
			Tags:                []string{"docker", "chrome", "cft", "startup", "health"},
			Command:             []string{"bash", "scripts/docker-chrome-cft-smoke.sh", chromeImage},
			RequiresChromeImage: true,
		},
		dockerSmokeStep{
			Name:                "docker: instance port conflict detection",
			Tags:                []string{"docker", "chrome", "cft", "ports", "conflict"},
			Command:             []string{"bash", "scripts/docker-port-conflict-smoke.sh", chromeImage},
			RequiresChromeImage: true,
		},
		dockerSmokeStep{
			Name:                 "docker: MCP stdio in container",
			Tags:                 []string{"docker", "mcp", "stdio", "release", "container"},
			Command:              []string{"bash", "scripts/docker-mcp-smoke.sh", releaseImage},
			RequiresReleaseImage: true,
		},
	)
	return dockerSmokePlan{steps: steps, images: []smokeImage{release, chrome}}
}

func selectDockerSmokeSteps(steps []dockerSmokeStep, filter string) []dockerSmokeStep {
	if filter == "" {
		return steps
	}

	selected := map[int]bool{}
	for i, step := range steps {
		if dockerSmokeStepMatchesFilter(step, filter) {
			selected[i] = true
		}
	}
	if len(selected) == 0 {
		return nil
	}

	needsReleaseImage := false
	needsChromeImage := false
	for i := range selected {
		needsReleaseImage = needsReleaseImage || steps[i].RequiresReleaseImage
		needsChromeImage = needsChromeImage || steps[i].RequiresChromeImage
	}
	if needsReleaseImage {
		for i, step := range steps {
			if step.ProvidesReleaseImage {
				selected[i] = true
				break
			}
		}
	}
	if needsChromeImage {
		for i, step := range steps {
			if step.ProvidesChromeImage {
				selected[i] = true
				break
			}
		}
	}

	out := make([]dockerSmokeStep, 0, len(selected))
	for i, step := range steps {
		if selected[i] {
			out = append(out, step)
		}
	}
	return out
}

func dockerSmokeStepMatchesFilter(step dockerSmokeStep, filter string) bool {
	for _, value := range append([]string{step.Name}, step.Tags...) {
		if strings.Contains(value, filter) {
			return true
		}
	}
	return false
}
