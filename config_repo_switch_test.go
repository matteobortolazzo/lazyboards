package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// retargetedProvider is a BoardProvider that serves one fixed board. It embeds
// *provider.FakeProvider only to inherit the rest of the interface; FetchBoard
// is overridden so a test can tell which provider a fetch actually went
// through by looking at the cards that come back.
type retargetedProvider struct {
	*provider.FakeProvider
	board provider.Board
}

func (p *retargetedProvider) FetchBoard(ctx context.Context) (provider.Board, error) {
	return p.board, nil
}

// singleCardBoard builds a one-column board holding a single card, so a
// fetch's origin is identifiable by card number alone.
func singleCardBoard(colTitle string, number int, title string) provider.Board {
	return provider.Board{
		Columns: []provider.Column{
			{Title: colTitle, Cards: []provider.Card{{Number: number, Title: title}}},
		},
	}
}

// newRepoSwitchBoard returns a loaded board tracking oldOwner/oldRepo whose
// providerFactory hands out a distinct provider serving newRepoCard, plus the
// recorded factory calls.
func newRepoSwitchBoard(t *testing.T, newRepoBoard provider.Board) (Board, *[]string, *retargetedProvider) {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "old-owner", "old-repo", "github", 0, 0, "Working", false, false, nil, nil, true)

	var calls []string
	next := &retargetedProvider{FakeProvider: provider.NewFakeProvider(), board: newRepoBoard}
	b.providerFactory = func(providerName, owner, repo string) (provider.BoardProvider, error) {
		calls = append(calls, providerName+":"+owner+"/"+repo)
		return next, nil
	}

	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	return m.(Board), &calls, next
}

// runCmd executes a tea.Cmd (flattening tea.Batch) and returns every non-batch
// message produced, skipping commands that block (e.g. tea.Tick).
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, sub := range batch {
				out = append(out, runCmd(sub)...)
			}
			return out
		}
		return []tea.Msg{msg}
	case <-time.After(200 * time.Millisecond):
		return nil
	}
}

// --- Retargeting the board at the newly saved repository ---

func TestConfigSaved_RepoChange_AdoptsNewOwnerAndRepo(t *testing.T) {
	b, calls, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))

	m, cmd := b.Update(configSavedMsg{provider: "github", repo: "new-owner/new-repo"})
	b = m.(Board)

	if b.repoOwner != "new-owner" || b.repoName != "new-repo" {
		t.Errorf("after saving new-owner/new-repo: repoOwner/repoName = %q/%q, want %q/%q",
			b.repoOwner, b.repoName, "new-owner", "new-repo")
	}
	if len(*calls) != 1 || (*calls)[0] != "github:new-owner/new-repo" {
		t.Errorf("provider factory calls = %v, want one call for %q", *calls, "github:new-owner/new-repo")
	}
	if b.mode != loadingMode {
		t.Errorf("mode = %d after a repo change, want %d (loadingMode)", b.mode, loadingMode)
	}
	if cmd == nil {
		t.Fatal("a repo change should return a fetch cmd")
	}
}

func TestConfigSaved_RepoChange_FetchesFromTheNewRepo(t *testing.T) {
	const newRepoCardNumber = 900
	b, _, _ := newRepoSwitchBoard(t, singleCardBoard("New", newRepoCardNumber, "New repo card"))

	m, cmd := b.Update(configSavedMsg{provider: "github", repo: "new-owner/new-repo"})
	b = m.(Board)

	var fetched *boardFetchedMsg
	for _, msg := range runCmd(cmd) {
		if bf, ok := msg.(boardFetchedMsg); ok {
			fetched = &bf
		}
	}
	if fetched == nil {
		t.Fatal("the post-save fetch produced no boardFetchedMsg")
	}
	if len(fetched.board.Columns) != 1 || len(fetched.board.Columns[0].Cards) != 1 {
		t.Fatalf("fetched board = %+v, want the single-card board served by the new repo's provider", fetched.board)
	}
	if got := fetched.board.Columns[0].Cards[0].Number; got != newRepoCardNumber {
		t.Errorf("fetched card number = %d, want %d -- the fetch went to the old repo's provider", got, newRepoCardNumber)
	}

	// And the board must actually render the new repo's cards.
	m, _ = b.Update(*fetched)
	b = m.(Board)
	if len(b.Columns) != 1 || len(b.Columns[0].Cards) != 1 ||
		b.Columns[0].Cards[0].Number != newRepoCardNumber {
		t.Errorf("board columns after the switch = %+v, want only card #%d", b.Columns, newRepoCardNumber)
	}
}

func TestConfigSaved_ProviderChange_RebuildsProvider(t *testing.T) {
	b, calls, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))

	// Same repo, different provider: still a retarget.
	m, _ := b.Update(configSavedMsg{provider: "azure-devops", repo: "old-owner/old-repo"})
	b = m.(Board)

	if b.providerName != "azure-devops" {
		t.Errorf("providerName = %q after saving a new provider, want %q", b.providerName, "azure-devops")
	}
	if len(*calls) != 1 || (*calls)[0] != "azure-devops:old-owner/old-repo" {
		t.Errorf("provider factory calls = %v, want one call for %q", *calls, "azure-devops:old-owner/old-repo")
	}
}

func TestConfigSaved_SameRepo_KeepsExistingProvider(t *testing.T) {
	b, calls, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))
	before := b.provider

	m, cmd := b.Update(configSavedMsg{provider: "github", repo: "old-owner/old-repo"})
	b = m.(Board)

	if len(*calls) != 0 {
		t.Errorf("provider factory calls = %v, want none when the repo is unchanged", *calls)
	}
	if b.provider != before {
		t.Error("an unchanged repo should keep the existing provider")
	}
	if b.mode != loadingMode {
		t.Errorf("mode = %d after saving an unchanged repo, want %d (loadingMode)", b.mode, loadingMode)
	}
	if cmd == nil {
		t.Error("saving an unchanged repo should still reload the board")
	}
}

// --- Stale state from the previous repo ---

func TestConfigSaved_RepoChange_DoesNotRunCleanupForOldRepoCards(t *testing.T) {
	// Every card of the old repo is absent from the new repo's board. Without
	// dropping prevCards on a switch, each one reads as a departure from its
	// column and fires that column's cleanup command against a card that never
	// moved.
	//
	// The board sizes here are deliberate. The missing-card debounce defers a
	// departure to the second consecutive miss, so the new repo is fetched
	// twice -- one fetch alone cannot distinguish a cleared prevCards map from
	// a merely deferred departure. And only one old card sits in the
	// cleanup-configured column, so the would-be departure stays under the
	// cleanup circuit breaker, which would otherwise swallow a mass departure
	// and make this test pass for the wrong reason.
	cleanup := "echo cleanup {{card_number}}"
	fe := &action.FakeExecutor{}
	columnConfigs := []config.ColumnConfig{
		{Name: "New", Cleanup: &cleanup},
		{Name: "Refined"},
	}
	oldBoard := provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{{Number: 1, Title: "Old repo card"}}},
			{Title: "Refined", Cards: []provider.Card{{Number: 2, Title: "Old repo card 2"}}},
		},
	}
	newBoard := provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{{Number: 900, Title: "New repo card"}}},
			{Title: "Refined"},
		},
	}

	b := NewBoard(provider.NewFakeProvider(), nil, columnConfigs, fe, "old-owner", "old-repo", "github", 0, 0, "Working", false, false, nil, nil, true)
	next := &retargetedProvider{FakeProvider: provider.NewFakeProvider(), board: newBoard}
	b.providerFactory = func(providerName, owner, repo string) (provider.BoardProvider, error) {
		return next, nil
	}

	m, cmd := b.Update(boardFetchedMsg{board: oldBoard})
	b = m.(Board)
	execCmds(cmd)
	if len(b.prevCards) == 0 {
		t.Fatal("precondition: the initial load should populate prevCards")
	}
	fe.RunShellCalls = nil

	m, _ = b.Update(configSavedMsg{provider: "github", repo: "new-owner/new-repo"})
	b = m.(Board)
	for range 2 {
		m, cmd = b.Update(boardFetchedMsg{board: newBoard})
		b = m.(Board)
		execCmds(cmd)
	}

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("switching repos ran cleanup commands %v, want none -- old-repo cards are not departures", fe.RunShellCalls)
	}
	if b.cleanupBreakerWarning != "" {
		t.Errorf("cleanupBreakerWarning = %q, want empty -- old-repo cards should not reach the breaker as departures at all", b.cleanupBreakerWarning)
	}
}

func TestConfigSaved_RepoChange_ClearsPreviousRepoState(t *testing.T) {
	b, _, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))
	b.collaborators = []Assignee{{Login: "alice"}}
	b.authenticatedUser = "alice"
	b.repoLabels = []string{"infra", "docs"}
	b.lastMetadataFetch = time.Now()
	b.openPRCount = 7
	b.loadErr = "previous failure"

	m, _ := b.Update(configSavedMsg{provider: "github", repo: "new-owner/new-repo"})
	b = m.(Board)

	if b.collaborators != nil {
		t.Errorf("collaborators = %+v after a repo switch, want nil", b.collaborators)
	}
	if b.authenticatedUser != "" {
		t.Errorf("authenticatedUser = %q after a repo switch, want empty", b.authenticatedUser)
	}
	if b.repoLabels != nil {
		t.Errorf("repoLabels = %v after a repo switch, want nil", b.repoLabels)
	}
	if !b.lastMetadataFetch.IsZero() {
		t.Error("lastMetadataFetch should be reset so the new repo's metadata is not TTL-gated behind the old repo's fetch")
	}
	if !b.metadataDue() {
		t.Error("metadata should be due immediately after a repo switch")
	}
	if b.openPRCount != -1 {
		t.Errorf("openPRCount = %d after a repo switch, want -1 (never fetched)", b.openPRCount)
	}
	if b.loadErr != "" {
		t.Errorf("loadErr = %q after a repo switch, want empty", b.loadErr)
	}
	if len(b.Columns) != 0 {
		t.Errorf("Columns = %+v after a repo switch, want empty -- the old repo's cards must not linger", b.Columns)
	}
}

func TestConfigSaved_RepoChange_ClearsSearchAndFilter(t *testing.T) {
	b, _, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))
	b.searchQuery = "auth"
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "infra"
	b.ActiveTab = 2

	m, _ := b.Update(configSavedMsg{provider: "github", repo: "new-owner/new-repo"})
	b = m.(Board)

	if b.searchQuery != "" {
		t.Errorf("searchQuery = %q after a repo switch, want empty", b.searchQuery)
	}
	if b.activeFilterType != filterTypeNone {
		t.Errorf("activeFilterType = %d after a repo switch, want %d (none) -- the old repo's labels do not exist in the new one", b.activeFilterType, filterTypeNone)
	}
	if b.activeFilterValue != "" {
		t.Errorf("activeFilterValue = %q after a repo switch, want empty", b.activeFilterValue)
	}
	if b.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d after a repo switch, want 0", b.ActiveTab)
	}
}

// --- Failure paths ---

func TestConfigSaved_FactoryError_StaysInConfigModeWithError(t *testing.T) {
	b, _, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))
	before := b.provider
	b.providerFactory = func(providerName, owner, repo string) (provider.BoardProvider, error) {
		return nil, errors.New("unsupported provider \"azure-devops\"")
	}

	m, cmd := b.Update(configSavedMsg{provider: "azure-devops", repo: "new-owner/new-repo"})
	b = m.(Board)

	if b.mode != configMode {
		t.Errorf("mode = %d after a failed retarget, want %d (configMode)", b.mode, configMode)
	}
	if !strings.Contains(b.validationErr, "azure-devops") {
		t.Errorf("validationErr = %q, want it to report the provider failure", b.validationErr)
	}
	if b.provider != before {
		t.Error("a failed retarget must keep the previous provider")
	}
	if b.repoOwner != "old-owner" || b.repoName != "old-repo" {
		t.Errorf("repoOwner/repoName = %q/%q after a failed retarget, want the previous %q/%q",
			b.repoOwner, b.repoName, "old-owner", "old-repo")
	}
	if cmd != nil {
		t.Error("a failed retarget should not kick off a fetch")
	}
}

func TestConfigSaved_NoFactory_RepoChange_ReportsInsteadOfRefreshingOldRepo(t *testing.T) {
	b, _, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))
	b.providerFactory = nil

	m, cmd := b.Update(configSavedMsg{provider: "github", repo: "new-owner/new-repo"})
	b = m.(Board)

	if b.mode != configMode {
		t.Errorf("mode = %d with no provider factory, want %d (configMode)", b.mode, configMode)
	}
	if b.validationErr == "" {
		t.Error("with no provider factory a repo change must report that it cannot take effect, not silently refresh the old repo")
	}
	if cmd != nil {
		t.Error("with no provider factory a repo change should not refresh the old repo")
	}
	if b.repoOwner != "old-owner" || b.repoName != "old-repo" {
		t.Errorf("repoOwner/repoName = %q/%q, want the unchanged %q/%q", b.repoOwner, b.repoName, "old-owner", "old-repo")
	}
}

func TestConfigSaved_MalformedRepo_StaysInConfigModeWithError(t *testing.T) {
	b, calls, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))

	m, cmd := b.Update(configSavedMsg{provider: "github", repo: "justarepo"})
	b = m.(Board)

	if b.mode != configMode {
		t.Errorf("mode = %d for a malformed repo, want %d (configMode)", b.mode, configMode)
	}
	if b.validationErr == "" {
		t.Error("a malformed repo should set a validation error")
	}
	if len(*calls) != 0 {
		t.Errorf("provider factory calls = %v, want none for a malformed repo", *calls)
	}
	if cmd != nil {
		t.Error("a malformed repo should not kick off a fetch")
	}
}

// --- Enter-time validation in the config modal ---

func TestConfigMode_Enter_RepoWithoutOwner_ShowsValidationError(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b.config.repoInput.SetValue("justarepo")

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	if b.mode != configMode {
		t.Errorf("mode = %d after Enter with a malformed repo, want %d (configMode)", b.mode, configMode)
	}
	if b.validationErr == "" {
		t.Error("Enter with a repo missing its owner should show a validation error")
	}
	if cmd != nil {
		t.Error("Enter with a malformed repo should not save the config")
	}
}

func TestConfigMode_Enter_RepoWithEmptyOwner_ShowsValidationError(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b.config.repoInput.SetValue("/repo")

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	if b.validationErr == "" {
		t.Error("Enter with an empty owner should show a validation error")
	}
	if cmd != nil {
		t.Error("Enter with an empty owner should not save the config")
	}
}

func TestConfigMode_Enter_ValidRepo_CarriesProviderAndRepoToSavedMsg(t *testing.T) {
	// The saved provider/repo must reach the handler, otherwise it has nothing
	// to retarget the board with.
	b := newLoadedTestBoard(t)
	b.config.localPath = t.TempDir() + "/.lazyboards.yml"
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b.config.repoInput.SetValue("new-owner/new-repo")

	_, cmd := b.Update(arrowMsg(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("Enter with a valid repo should return a save cmd")
	}

	var saved *configSavedMsg
	for _, msg := range runCmd(cmd) {
		if cs, ok := msg.(configSavedMsg); ok {
			saved = &cs
		}
	}
	if saved == nil {
		t.Fatal("the save cmd produced no configSavedMsg")
	}
	if saved.repo != "new-owner/new-repo" {
		t.Errorf("configSavedMsg.repo = %q, want %q", saved.repo, "new-owner/new-repo")
	}
	if saved.provider != "github" {
		t.Errorf("configSavedMsg.provider = %q, want %q", saved.provider, "github")
	}
}
