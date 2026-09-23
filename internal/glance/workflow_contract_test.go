package glance

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflowContract struct {
	On struct {
		Push struct {
			Branches []string `yaml:"branches"`
			Tags     []string `yaml:"tags"`
		} `yaml:"push"`
		PullRequest struct {
			Branches []string `yaml:"branches"`
		} `yaml:"pull_request"`
	} `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		If          string            `yaml:"if"`
		Needs       string            `yaml:"needs"`
		Permissions map[string]string `yaml:"permissions"`
		Steps       []struct {
			Uses string `yaml:"uses"`
			Run  string `yaml:"run"`
			With struct {
				GoVersionFile      string `yaml:"go-version-file"`
				PersistCredentials *bool  `yaml:"persist-credentials"`
			} `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func loadWorkflowContract(t *testing.T, filename string) workflowContract {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", filename))
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowContract
	if err := yaml.Unmarshal(contents, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func TestForkWorkflowContracts(t *testing.T) {
	ci := loadWorkflowContract(t, "fork-ci.yaml")
	if !reflect.DeepEqual(ci.On.Push.Branches, []string{"main"}) ||
		!reflect.DeepEqual(ci.On.PullRequest.Branches, []string{"main"}) ||
		len(ci.On.Push.Tags) != 0 {
		t.Fatal("CI must run for pushes and PRs to main, not tags")
	}
	if !reflect.DeepEqual(ci.Permissions, map[string]string{"contents": "read"}) {
		t.Fatalf("CI permissions: %v", ci.Permissions)
	}
	ciJob := ci.Jobs["verify"]
	if ciJob.If != "github.repository == 'ChatArch/glance'" {
		t.Fatalf("CI repository guard: %q", ciJob.If)
	}
	ciSteps := ""
	for _, step := range ciJob.Steps {
		ciSteps += step.Run + "\n"
		if strings.HasPrefix(step.Uses, "actions/setup-go@") && step.With.GoVersionFile != "go.mod" {
			t.Fatal("CI must use the go.mod toolchain")
		}
	}
	for _, command := range []string{
		"go test ./... -count=1", "go test -race ./internal/glance -run '^TestPageVisibility' -count=1",
		"go vet ./...", "go build ./...",
	} {
		if !strings.Contains(ciSteps, command) {
			t.Errorf("missing CI gate %q", command)
		}
	}

	release := loadWorkflowContract(t, "fork-release.yaml")
	if !reflect.DeepEqual(release.On.Push.Tags, []string{"chatarch-v*"}) || len(release.On.Push.Branches) != 0 {
		t.Fatal("fork releases must trigger only on chatarch-v* tags")
	}
	if !reflect.DeepEqual(release.Permissions, map[string]string{"contents": "read"}) {
		t.Fatalf("release default permissions: %v", release.Permissions)
	}
	verification := release.Jobs["verify-and-build"]
	publication := release.Jobs["publish"]
	if verification.If != "github.repository == 'ChatArch/glance'" ||
		publication.If != verification.If || publication.Needs != "verify-and-build" {
		t.Fatal("fork verification and publication must be repository-guarded and ordered")
	}
	if len(verification.Permissions) != 0 ||
		!reflect.DeepEqual(publication.Permissions, map[string]string{"contents": "write"}) {
		t.Fatal("only the publish job may request contents:write")
	}
	verificationSteps := ""
	for _, step := range verification.Steps {
		verificationSteps += step.Run + "\n"
		if step.Uses == "actions/checkout@v4" &&
			(step.With.PersistCredentials == nil || *step.With.PersistCredentials) {
			t.Fatal("verification checkout must not persist credentials")
		}
		if strings.HasPrefix(step.Uses, "actions/setup-go@") && step.With.GoVersionFile != "go.mod" {
			t.Fatal("release verification must use the go.mod toolchain")
		}
	}
	for _, condition := range []string{
		`^chatarch-v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`,
		`git merge-base --is-ancestor "$tag_commit" refs/remotes/origin/main`,
		`git rev-parse "refs/tags/${TAG}^{commit}"`,
		"go test ./... -count=1", "go test -race ./internal/glance -run '^TestPageVisibility' -count=1",
		"go vet ./...", `version="${TAG}+${commit}"`,
		"github.com/glanceapp/glance/internal/glance.buildVersion=${version}",
		`test "$(dist/glance --version)" = "$version"`,
		"sha256sum", "go build", "GOOS=linux GOARCH=amd64",
	} {
		if !strings.Contains(verificationSteps, condition) {
			t.Errorf("missing release gate %q", condition)
		}
	}
	if strings.Index(verificationSteps, "go test ./... -count=1") > strings.Index(verificationSteps, "go build") {
		t.Fatal("release must test before building")
	}
	publicationSteps := ""
	publicationCommands := ""
	for _, step := range publication.Steps {
		publicationSteps += step.Uses + "\n"
		publicationCommands += step.Run + "\n"
		if step.Uses == "actions/checkout@v4" &&
			(step.With.PersistCredentials == nil || *step.With.PersistCredentials) {
			t.Fatal("publication checkout must not persist credentials")
		}
	}
	if !strings.Contains(publicationSteps, "actions/download-artifact@v4") ||
		!strings.Contains(publicationSteps, "softprops/action-gh-release@v2") {
		t.Fatal("only the publish job should download and release the verified archive")
	}
	for _, condition := range []string{
		`git fetch --force --no-tags origin "+refs/tags/${TAG}:refs/tags/${TAG}"`,
		`git merge-base --is-ancestor "$tag_commit" refs/remotes/origin/main`,
		"sha256sum -c SHA256SUMS",
	} {
		if !strings.Contains(publicationCommands, condition) {
			t.Errorf("missing publication recheck %q", condition)
		}
	}
	if strings.Contains(verificationSteps+publicationSteps, "goreleaser") ||
		strings.Contains(verificationSteps+publicationSteps, "docker/login-action") {
		t.Fatal("fork release must not publish upstream Docker images")
	}

	upstream := loadWorkflowContract(t, "release.yaml")
	if !reflect.DeepEqual(upstream.On.Push.Tags, []string{"v*"}) ||
		upstream.Jobs["release"].If != "github.repository == 'glanceapp/glance'" {
		t.Fatal("the inherited v* release must be restricted to the upstream repository")
	}
}
