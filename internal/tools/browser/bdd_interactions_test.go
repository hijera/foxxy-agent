//go:build browser

package browser

// Godog harness for features/browser_interactions.feature: the happy path of the
// interaction tools (press_key, wait_for, select_option, handle_dialog,
// fill_form, route mocks, tabs) against a real Chrome and a real HTTP server.

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

type interactionsState struct {
	t *testing.T

	m   *Manager
	env *tooling.Env
	srv *httptest.Server

	lastResult string
	optionList string
	tabList    string
}

func (s *interactionsState) reset() error {
	s.closeAll()
	s.m = NewManager(&config.BrowserConfig{Enabled: true})
	s.env = &tooling.Env{
		SessionID:    "bdd-interactions",
		SessionDir:   s.t.TempDir(),
		AddToolImage: func(_, _, _ string) {},
	}
	s.lastResult, s.optionList, s.tabList = "", "", ""
	if s.srv == nil {
		s.srv = interactionsServer(s.t)
	}
	return s.exec("navigate", s.m.executeNavigate, `{"url":"`+s.srv.URL+`"}`)
}

func (s *interactionsState) closeAll() {
	if s.m != nil {
		s.m.closeSession("bdd-interactions")
	}
}

// exec runs a tool and fails on an error result, keeping the output for the
// Then steps.
func (s *interactionsState) exec(label string, tool func(context.Context, string, *tooling.Env) (string, error), argsJSON string) error {
	out, err := tool(context.Background(), argsJSON, s.env)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if strings.HasPrefix(out, "error:") {
		return fmt.Errorf("%s reported %q", label, out)
	}
	s.lastResult = out
	return nil
}

// pageText reads a DOM string through the evaluate tool.
func (s *interactionsState) pageText(expr string) (string, error) {
	out, err := s.m.executeEvaluate(context.Background(), `{"expression":"`+expr+`"}`, s.env)
	if err != nil {
		return "", err
	}
	raw, ok := strings.CutPrefix(out, "result: ")
	if !ok {
		return "", fmt.Errorf("evaluate returned %q", out)
	}
	return strings.Trim(raw, `"`), nil
}

func (s *interactionsState) fillAndPressEnter() error {
	if err := s.exec("fill", s.m.executeFill, `{"selector":"#q","text":"dune"}`); err != nil {
		return err
	}
	return s.exec("press_key", s.m.executePressKey, `{"key":"Enter","selector":"#q"}`)
}

func (s *interactionsState) pageShowsSubmission() error {
	got, err := s.pageText(`document.getElementById('submitted').textContent`)
	if err != nil {
		return err
	}
	if got != "sent:dune" {
		return fmt.Errorf("submitted = %q, want \"sent:dune\"", got)
	}
	return nil
}

func (s *interactionsState) waitForLateContent() error {
	return s.exec("wait_for", s.m.executeWaitFor, `{"text":"late content arrived","seconds":10}`)
}

func (s *interactionsState) waitReportedConditionMet() error {
	if !strings.Contains(s.lastResult, "condition met") {
		return fmt.Errorf("wait_for answer does not report the condition: %s", s.lastResult)
	}
	return nil
}

func (s *interactionsState) listPetOptions() error {
	if err := s.exec("select_option list", s.m.executeSelectOption, `{"selector":"#pet"}`); err != nil {
		return err
	}
	s.optionList = s.lastResult
	return nil
}

func (s *interactionsState) listingNamesCatAndDog() error {
	for _, want := range []string{`label="Cat"`, `label="Dog"`} {
		if !strings.Contains(s.optionList, want) {
			return fmt.Errorf("options listing is missing %s:\n%s", want, s.optionList)
		}
	}
	return nil
}

func (s *interactionsState) pickDogByLabel() error {
	return s.exec("select_option pick", s.m.executeSelectOption, `{"selector":"#pet","label":"Dog"}`)
}

func (s *interactionsState) changeHandlerSawDog() error {
	got, err := s.pageText(`document.getElementById('petout').textContent`)
	if err != nil {
		return err
	}
	if got != "dog" {
		return fmt.Errorf("petout = %q, want \"dog\"", got)
	}
	return nil
}

func (s *interactionsState) armDialogAccept() error {
	return s.exec("handle_dialog", s.m.executeHandleDialog, `{"accept":true}`)
}

func (s *interactionsState) clickAskButton() error {
	return s.exec("click ask", s.m.executeClick, `{"selector":"#ask"}`)
}

func (s *interactionsState) verdictIs(want string) error {
	got, err := s.pageText(`document.getElementById('verdict').textContent`)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("verdict = %q, want %q", got, want)
	}
	return nil
}

func (s *interactionsState) pageLogRecordsDialog() error {
	out, err := s.m.executePageLog(context.Background(), "{}", s.env)
	if err != nil {
		return err
	}
	if !strings.Contains(s.lastResult+out, "[dialog]") {
		return fmt.Errorf("no [dialog] line anywhere:\naction: %s\nlog: %s", s.lastResult, out)
	}
	return nil
}

func (s *interactionsState) fillWholeForm() error {
	return s.exec("fill_form", s.m.executeFillForm, `{"fields":[
		{"selector":"#first","value":"hello"},
		{"selector":"#notes","value":"world"},
		{"selector":"#agree","value":"true"},
		{"selector":"#pet","value":"cat"}
	]}`)
}

func (s *interactionsState) everyFieldHoldsItsValue() error {
	checks := []struct{ expr, want string }{
		{`document.getElementById('first').value`, "hello"},
		{`document.getElementById('notes').value`, "world"},
		{`String(document.getElementById('agree').checked)`, "true"},
		{`document.getElementById('pet').value`, "cat"},
	}
	for _, c := range checks {
		got, err := s.pageText(c.expr)
		if err != nil {
			return err
		}
		if got != c.want {
			return fmt.Errorf("%s = %q, want %q", c.expr, got, c.want)
		}
	}
	return nil
}

func (s *interactionsState) mockDataAPI() error {
	return s.exec("route mock", s.m.executeRoute,
		`{"action":"mock","url_substring":"/api/data","status":200,"content_type":"application/json","body":"{\"answer\":42}"}`)
}

func (s *interactionsState) navigateToDataAPI() error {
	return s.exec("navigate api", s.m.executeNavigate, `{"url":"`+s.srv.URL+`/api/data"}`)
}

func (s *interactionsState) pageShowsCannedAnswer() error {
	got, err := s.pageText(`document.body.innerText`)
	if err != nil {
		return err
	}
	if !strings.Contains(got, "42") {
		return fmt.Errorf("mocked body did not reach the page: %q", got)
	}
	return nil
}

func (s *interactionsState) openSecondPageInNewTab() error {
	return s.exec("tabs new", s.m.executeTabs, `{"action":"new","url":"`+s.srv.URL+`/second"}`)
}

func (s *interactionsState) tabListShowsTwoTabs() error {
	if err := s.exec("tabs list", s.m.executeTabs, `{"action":"list"}`); err != nil {
		return err
	}
	s.tabList = s.lastResult
	if !strings.Contains(s.tabList, "2 tab(s)") {
		return fmt.Errorf("tab list does not show two tabs:\n%s", s.tabList)
	}
	return nil
}

func (s *interactionsState) switchToFirstTab() error {
	num := 0
	for _, line := range strings.Split(s.tabList, "\n") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimLeft(line, "* "), "tab %d:", &n); err != nil {
			continue
		}
		_, rest, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		lineURL, _, _ := strings.Cut(rest, " — ")
		if strings.TrimSpace(lineURL) == s.srv.URL+"/" {
			num = n
			break
		}
	}
	if num == 0 {
		return fmt.Errorf("the first page is not in the tab list:\n%s", s.tabList)
	}
	return s.exec("tabs select", s.m.executeTabs, fmt.Sprintf(`{"action":"select","tab":%d}`, num))
}

func (s *interactionsState) currentPageIsFirstPage() error {
	got, err := s.pageText(`location.pathname`)
	if err != nil {
		return err
	}
	if got != "/" {
		return fmt.Errorf("current pathname = %q, want \"/\"", got)
	}
	return nil
}

func initializeInteractionsScenario(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &interactionsState{t: t}
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			s.closeAll()
			return ctx, nil
		})
		sc.Step(`^a browser session on the interactions page$`, s.reset)
		sc.Step(`^I fill the search field and press Enter in it$`, s.fillAndPressEnter)
		sc.Step(`^the page shows what was submitted$`, s.pageShowsSubmission)
		sc.Step(`^I wait for the late content text$`, s.waitForLateContent)
		sc.Step(`^the wait reports the condition was met$`, s.waitReportedConditionMet)
		sc.Step(`^I list the options of the pet dropdown$`, s.listPetOptions)
		sc.Step(`^the listing names the Cat and Dog options$`, s.listingNamesCatAndDog)
		sc.Step(`^I pick the Dog option by label$`, s.pickDogByLabel)
		sc.Step(`^the page's change handler saw the dog value$`, s.changeHandlerSawDog)
		sc.Step(`^I arm the next dialog to be accepted$`, s.armDialogAccept)
		sc.Step(`^I click the button that asks for confirmation$`, s.clickAskButton)
		sc.Step(`^the page shows the accepted verdict$`, func() error { return s.verdictIs("yes") })
		sc.Step(`^the page shows the dismissed verdict$`, func() error { return s.verdictIs("no") })
		sc.Step(`^the page log records the dialog$`, s.pageLogRecordsDialog)
		sc.Step(`^I fill the whole form in one call$`, s.fillWholeForm)
		sc.Step(`^every field holds its value$`, s.everyFieldHoldsItsValue)
		sc.Step(`^I mock the data API with a canned answer$`, s.mockDataAPI)
		sc.Step(`^I navigate to the data API path$`, s.navigateToDataAPI)
		sc.Step(`^the page shows the canned answer$`, s.pageShowsCannedAnswer)
		sc.Step(`^I open the second page in a new tab$`, s.openSecondPageInNewTab)
		sc.Step(`^the tab list shows two tabs$`, s.tabListShowsTwoTabs)
		sc.Step(`^I switch back to the first tab$`, s.switchToFirstTab)
		sc.Step(`^the current page is the first page$`, s.currentPageIsFirstPage)
	}
}

func TestBrowserInteractionsFeature(t *testing.T) {
	// Skip cleanly where no Chrome is installed, like the other browser tests.
	newTestManager(t)

	suite := godog.TestSuite{
		Name:                "browser-interactions",
		ScenarioInitializer: initializeInteractionsScenario(t),
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../../features/browser_interactions.feature"},
			TestingT: t,
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("browser interactions feature suite failed")
	}
}
