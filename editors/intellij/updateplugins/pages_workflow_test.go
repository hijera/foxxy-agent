// Tests for how GitHub Pages gets redeployed after the plugin repository document changes.
//
// Pages is built by .github/workflows/site.yaml, which publishes the website with the whole docs/
// tree laid under it. docs/updatePlugins.xml is committed to main by the release pipeline with
// GITHUB_TOKEN, and such a push never starts another workflow, so nothing would deploy the new
// document unless the release pipeline (and the manual rollback) call site.yaml themselves. JetBrains
// IDEs poll that address for updates: a missed deploy is a release no IDE is offered.
package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	workflowsDir             = "../../../.github/workflows"
	siteWorkflowUses         = "./.github/workflows/site.yaml"
	pluginRepositoryWorkflow = "plugin-repository.yaml"
	tagOnMergeWorkflow       = "tag-on-merge.yaml"
)

type pagesWorkflow struct {
	Jobs map[string]pagesJob `yaml:"jobs"`
}

type pagesJob struct {
	Needs       pagesNeeds        `yaml:"needs"`
	If          string            `yaml:"if"`
	Uses        string            `yaml:"uses"`
	Permissions map[string]string `yaml:"permissions"`
	Environment any               `yaml:"environment"`
	Steps       []pagesStep       `yaml:"steps"`
}

type pagesStep struct {
	Name string         `yaml:"name"`
	ID   string         `yaml:"id"`
	If   string         `yaml:"if"`
	Uses string         `yaml:"uses"`
	Run  string         `yaml:"run"`
	With map[string]any `yaml:"with"`
}

// pagesNeeds is a job's needs:, a single id or a list.
type pagesNeeds []string

func (n *pagesNeeds) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*n = pagesNeeds{value.Value}
		return nil
	}
	var ids []string
	if err := value.Decode(&ids); err != nil {
		return err
	}
	*n = ids
	return nil
}

func readPagesWorkflow(t *testing.T, name string) pagesWorkflow {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workflowsDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var workflow pagesWorkflow
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return workflow
}

// siteJob returns the id and job that call site.yaml, failing when there is none.
func siteJob(t *testing.T, workflowName string, workflow pagesWorkflow) (string, pagesJob) {
	t.Helper()
	for id, job := range workflow.Jobs {
		if job.Uses == siteWorkflowUses {
			return id, job
		}
	}
	t.Fatalf("%s never calls site.yaml, so GitHub Pages keeps serving the previous updatePlugins.xml", workflowName)
	return "", pagesJob{}
}

func wantPagesPermissions(t *testing.T, where string, job pagesJob) {
	t.Helper()
	for _, scope := range []string{"pages", "id-token"} {
		if job.Permissions[scope] != "write" {
			t.Errorf("%s: permissions.%s is %q; actions/deploy-pages needs write", where, scope, job.Permissions[scope])
		}
	}
}

func TestReleasePipelineRedeploysPagesAfterThePluginRepositoryPublish(t *testing.T) {
	pipeline := readPagesWorkflow(t, tagOnMergeWorkflow)
	id, job := siteJob(t, tagOnMergeWorkflow, pipeline)

	for _, child := range []string{"intellij-plugin.yaml", "release-binaries.yaml", "vscode-plugin.yaml"} {
		builder := ""
		for otherID, other := range pipeline.Jobs {
			if other.Uses == "./.github/workflows/"+child {
				builder = otherID
			}
		}
		if builder == "" {
			t.Fatalf("%s no longer calls %s; update this test to the new release pipeline", tagOnMergeWorkflow, child)
		}
		if !slices.Contains(job.Needs, builder) {
			t.Errorf("job %s does not wait for %s (%s): Pages could deploy before updatePlugins.xml is committed or before the download links exist", id, builder, child)
		}
	}
	if !strings.Contains(job.If, "!cancelled()") && !strings.Contains(job.If, "always()") {
		t.Errorf("job %s runs only when every build passed (if: %q); a failed VS Code build would then hold back the IntelliJ update", id, job.If)
	}
	wantPagesPermissions(t, tagOnMergeWorkflow+" job "+id, job)
}

func TestManualPluginRepositoryPublishRedeploysPages(t *testing.T) {
	workflow := readPagesWorkflow(t, pluginRepositoryWorkflow)
	id, job := siteJob(t, pluginRepositoryWorkflow, workflow)

	publisher := ""
	for otherID, other := range workflow.Jobs {
		for _, step := range other.Steps {
			if step.Name == publishStep {
				publisher = otherID
			}
		}
	}
	if publisher == "" {
		t.Fatalf("%s has no step %q", pluginRepositoryWorkflow, publishStep)
	}
	if !slices.Contains(job.Needs, publisher) {
		t.Errorf("job %s does not wait for %s, so Pages could deploy the document from before the rollback", id, publisher)
	}
	wantPagesPermissions(t, pluginRepositoryWorkflow+" job "+id, job)
}

func TestSiteWorkflowPublishesTheDocsTree(t *testing.T) {
	workflow := readPagesWorkflow(t, "site.yaml")
	if len(workflow.Jobs) != 1 {
		t.Fatalf("site.yaml has %d jobs; build and deploy share one job so a slow older build cannot deploy after a newer one", len(workflow.Jobs))
	}
	for id, job := range workflow.Jobs {
		if env, ok := job.Environment.(map[string]any); !ok || env["name"] != "github-pages" {
			if job.Environment != "github-pages" {
				t.Errorf("job %s does not deploy to the github-pages environment (%v)", id, job.Environment)
			}
		}

		var checkout, assemble, upload, deploy *pagesStep
		buildID := ""
		for i := range job.Steps {
			step := &job.Steps[i]
			switch {
			case strings.HasPrefix(step.Uses, "actions/checkout"):
				checkout = step
			case strings.Contains(step.Run, "scripts/assemble.mjs"):
				assemble = step
			case strings.HasPrefix(step.Uses, "actions/upload-pages-artifact"):
				upload = step
			case strings.HasPrefix(step.Uses, "actions/deploy-pages"):
				deploy = step
			case strings.Contains(step.Run, "npm run build"):
				buildID = step.ID
			}
			if strings.Contains(step.Run, "gh release upload") || strings.Contains(step.Run, "gh release create") {
				t.Errorf("step %q touches a release; site.yaml must only read releases", step.Name)
			}
		}
		if checkout == nil || checkout.With["ref"] != "main" {
			t.Errorf("site.yaml must check out main, where the release pipeline has just committed updatePlugins.xml (not the merge commit that started the run)")
		}
		if assemble == nil || !strings.Contains(assemble.Run, "--docs docs") {
			t.Errorf("site.yaml must assemble the artifact from docs/ (scripts/assemble.mjs --docs docs), or updatePlugins.xml and config.schema.json disappear from Pages")
		}
		if upload == nil || upload.With["path"] != "_site" {
			t.Errorf("site.yaml must upload the assembled _site directory")
		}
		if deploy == nil {
			t.Fatalf("site.yaml never runs actions/deploy-pages")
		}
		if buildID != "" && strings.Contains(deploy.If, "steps."+buildID) {
			t.Errorf("the deploy depends on the site build (if: %q); a broken landing page must not hold back docs/", deploy.If)
		}
	}
}
