package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v68/github"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/auth"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

// gitHubClient combines GitHub API services into a single client satisfying provider.GitHubClient.
type gitHubClient struct {
	issues *github.IssuesService
	repos  *github.RepositoriesService
	users  *github.UsersService
}

func (c *gitHubClient) ListByRepo(ctx context.Context, owner string, repo string, opts *github.IssueListByRepoOptions) ([]*github.Issue, *github.Response, error) {
	return c.issues.ListByRepo(ctx, owner, repo, opts)
}

func (c *gitHubClient) Create(ctx context.Context, owner string, repo string, issue *github.IssueRequest) (*github.Issue, *github.Response, error) {
	return c.issues.Create(ctx, owner, repo, issue)
}

func (c *gitHubClient) Edit(ctx context.Context, owner string, repo string, number int, issue *github.IssueRequest) (*github.Issue, *github.Response, error) {
	return c.issues.Edit(ctx, owner, repo, number, issue)
}

func (c *gitHubClient) CreateLabel(ctx context.Context, owner string, repo string, label *github.Label) (*github.Label, *github.Response, error) {
	return c.issues.CreateLabel(ctx, owner, repo, label)
}

func (c *gitHubClient) ListLabels(ctx context.Context, owner string, repo string, opts *github.ListOptions) ([]*github.Label, *github.Response, error) {
	return c.issues.ListLabels(ctx, owner, repo, opts)
}

func (c *gitHubClient) ListCollaborators(ctx context.Context, owner string, repo string, opts *github.ListCollaboratorsOptions) ([]*github.User, *github.Response, error) {
	return c.repos.ListCollaborators(ctx, owner, repo, opts)
}

func (c *gitHubClient) GetUser(ctx context.Context, user string) (*github.User, *github.Response, error) {
	return c.users.Get(ctx, user)
}

func (c *gitHubClient) CreateComment(ctx context.Context, owner string, repo string, number int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	return c.issues.CreateComment(ctx, owner, repo, number, comment)
}

// lazyboardsRepoOwner and lazyboardsRepoName identify this project's own
// GitHub repo -- the fixed target for the startup update check, independent
// of whatever repo the user has configured the board to track issues/PRs
// for (b.repoOwner/b.repoName).
const (
	lazyboardsRepoOwner = "matteobortolazzo"
	lazyboardsRepoName  = "lazyboards"
)

// version is injected at release time via -ldflags "-X main.version=...".
// Empty in local builds; appVersion() then falls back to build info.
var version = ""

// appVersion resolves the running version: the injected ldflag value if set,
// otherwise the module version embedded by `go install` (ReadBuildInfo),
// otherwise "dev" for plain `go build`.
func appVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok &&
		info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// versionRequested reports whether CLI args ask to print the version and exit.
func versionRequested(args []string) bool {
	return len(args) > 1 &&
		(args[1] == "--version" || args[1] == "-v" || args[1] == "version")
}

// versionNewer reports whether latest is a newer version than current. Both
// operands may optionally start with a "v"/"V" prefix, which is stripped
// before comparison. Each version is split into dot-separated components and
// compared numerically component-by-component (not lexically, so 1.9.0 is
// correctly older than 1.10.0). If one version has fewer components but
// matches as a prefix, the shorter one is treated as older. Any non-numeric
// component in either operand fails safe to false (no false positive).
func versionNewer(current, latest string) bool {
	currentParts, ok := parseVersionComponents(current)
	if !ok {
		return false
	}
	latestParts, ok := parseVersionComponents(latest)
	if !ok {
		return false
	}

	n := len(currentParts)
	if len(latestParts) > n {
		n = len(latestParts)
	}
	for i := 0; i < n; i++ {
		var c, l int
		if i < len(currentParts) {
			c = currentParts[i]
		}
		if i < len(latestParts) {
			l = latestParts[i]
		}
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}
	return false
}

// parseVersionComponents strips a leading v/V and splits s on "." into
// numeric components. ok is false if any component fails to parse as an int.
func parseVersionComponents(s string) (parts []int, ok bool) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	for _, c := range strings.Split(s, ".") {
		n, err := strconv.Atoi(c)
		if err != nil {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}

// shouldCheckForUpdate reports whether the startup version-update check
// should run: never for "dev" builds (unreleased/local builds have nothing
// meaningful to compare), and never when the config has disabled it.
func shouldCheckForUpdate(version string, enabled bool) bool {
	return version != "dev" && enabled
}

// printNotices surfaces stderr notice groups (e.g. legacy-config deprecation
// notices from actions:/columns[].actions translated onto keymaps: (#510),
// and untrusted-local-config strip notices (#568)) once per run, before
// BubbleTea takes over the terminal. Each group's lines are printed in order,
// one sanitized line per entry; nil/empty groups contribute zero lines
// without disrupting the ordering of the surrounding groups.
func printNotices(w io.Writer, groups ...[]string) {
	for _, notices := range groups {
		for _, notice := range notices {
			_, _ = fmt.Fprintln(w, sanitizeSingleLine(notice))
		}
	}
}

func main() {
	if versionRequested(os.Args) {
		fmt.Printf("lazyboards %s\n", appVersion())
		return
	}

	// Dispatch "trust"/"untrust" before any config load or provider setup
	// (#568): these verbs only ever touch the local config file (read) and
	// the trust store (read/write), so they must run ahead of
	// debuglog.Init/config.DefaultGlobalPath/config.Load below, not thread
	// through the normal startup flow.
	if verb, ok := trustVerb(os.Args); ok {
		if trustVerbExtraArgs(os.Args) {
			fmt.Fprintf(os.Stderr, "Usage: lazyboards %s\n\n%q does not accept any extra arguments or flags.\n", verb, verb)
			os.Exit(1)
		}
		dispatchTrustPath, err := config.DefaultTrustPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving trust store path: %v\n", err)
			os.Exit(1)
		}
		note, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving working directory: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runTrustVerb(verb, config.DefaultLocalPath, dispatchTrustPath, note, os.Stderr))
	}

	// Open the debug log before anything else that might need to log to it
	// (e.g. the trust-store fallback below) -- Init only opens a file handle
	// keyed off an env var, so moving it ahead of config/trust resolution is
	// side-effect-free for everything after it.
	if err := debuglog.Init(os.Getenv("LAZYBOARDS_DEBUG_LOG")); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening debug log: %v\n", err)
	}

	globalPath, err := config.DefaultGlobalPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving config path: %v\n", err)
		os.Exit(1)
	}

	// Resolve the trust store once and reuse it for every config.Load call
	// below. Any error resolving/reading/parsing it (missing home dir,
	// unreadable or malformed trust.yml) fails closed to a zero-value Trust
	// -- which trusts nothing -- rather than aborting startup: a broken
	// trust store must never block the app, it must only ever narrow what
	// the local config is allowed to execute.
	trustPath, err := config.DefaultTrustPath()
	var trust config.Trust
	if err == nil {
		trust, err = config.LoadTrust(trustPath)
	}
	if err != nil {
		debuglog.Log(fmt.Sprintf("trust: falling back to zero-value trust store (nothing trusted): %v", err))
		trust = config.Trust{}
	}

	cfg, err := config.Load(globalPath, config.DefaultLocalPath, trust)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %s\n", sanitizeSingleLine(err.Error()))
		os.Exit(1)
	}

	// Auto-detect provider and repo from git remote. ResolveConfigPath
	// handles both a normal ".git" directory and a linked worktree's
	// ".git" gitdir-pointer file (see internal/git/remote.go).
	gitInfo := gitdetect.DetectRemote(gitdetect.ResolveConfigPath(".git"))

	// Config overrides git-detected values
	prov := cfg.Provider
	if prov == "" {
		prov = gitInfo.Provider
	}
	repo := cfg.Repo
	if repo == "" {
		repo = gitInfo.Repo
	}

	// Split repo early for reuse
	repoOwner, repoNameOnly := "", ""
	if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 {
		repoOwner = parts[0]
		repoNameOnly = parts[1]
	}

	// First-launch flow: show config popup when no local config exists
	// and git detection didn't provide both provider and repo.
	if !config.LocalExists(config.DefaultLocalPath) && (prov == "" || repo == "") {
		board := NewBoard(nil, nil, nil, nil, repoOwner, repoNameOnly, prov, 0, 0, config.DefaultWorkingLabel, false, true, nil, nil, cfg.UpdateCheckValue())
		p := tea.NewProgram(board, tea.WithAltScreen())
		m, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		b := m.(Board)
		if !b.config.configSaved {
			fmt.Fprintf(os.Stderr, "Configuration required. Exiting.\n")
			os.Exit(1)
		}
		// Reload config with saved values, reusing the same trust decision
		// resolved above (never reload the trust store twice).
		cfg, err = config.Load(globalPath, config.DefaultLocalPath, trust)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %s\n", sanitizeSingleLine(err.Error()))
			os.Exit(1)
		}
		prov = cfg.Provider
		if prov == "" {
			prov = gitInfo.Provider
		}
		repo = cfg.Repo
		if repo == "" {
			repo = gitInfo.Repo
		}
		repoOwner, repoNameOnly = "", ""
		if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 {
			repoOwner = parts[0]
			repoNameOnly = parts[1]
		}
	}

	var bp provider.BoardProvider
	// providerFactory lets the board rebuild its provider when the config
	// modal saves a different repository, so the switch takes effect without a
	// restart. It closes over the already-authenticated clients, so only the
	// owner/repo (and the column mapping, which the modal never changes) vary.
	var providerFactory func(providerName, owner, repo string) (provider.BoardProvider, error)
	switch prov {
	case "":
		fmt.Fprintf(os.Stderr, "No provider detected.\n\n")
		fmt.Fprintf(os.Stderr, "Ensure you are in a git repository with a GitHub or Azure DevOps remote,\n")
		fmt.Fprintf(os.Stderr, "or create a .lazyboards.yml with:\n\n")
		fmt.Fprintf(os.Stderr, "  provider: github\n")
		fmt.Fprintf(os.Stderr, "  repo: owner/repo\n\n")
		os.Exit(1)
	case "github":
		token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
		if token == "" {
			out, err := exec.Command("gh", "auth", "token").Output()
			if err == nil {
				token = strings.TrimSpace(string(out))
			}
		}
		if token == "" {
			fmt.Fprintf(os.Stderr, "GitHub token not found.\n\n")
			fmt.Fprintf(os.Stderr, "Either set GITHUB_TOKEN or authenticate with: gh auth login\n")
			os.Exit(1)
		}
		if err := auth.ValidateGitHubToken(token); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid GitHub token format.\n\n")
			fmt.Fprintf(os.Stderr, "Ensure GITHUB_TOKEN or `gh auth token` provides a valid token.\n")
			os.Exit(1)
		}
		cfgOwner, cfgRepoName, ok := splitRepo(repo)
		if !ok {
			fmt.Fprintf(os.Stderr, "Invalid repo format %q, expected \"owner/repo\"\n", repo)
			os.Exit(1)
		}
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(context.Background(), ts)
		ghClient := github.NewClient(tc)
		ghc := &gitHubClient{
			issues: ghClient.Issues,
			repos:  ghClient.Repositories,
			users:  ghClient.Users,
		}
		gqlClient := githubv4.NewClient(tc)
		gqlAdapter := provider.NewGitHubV4Adapter(gqlClient)
		columnNames := cfg.ColumnNames()
		providerFactory = func(providerName, owner, repo string) (provider.BoardProvider, error) {
			if providerName != "github" {
				return nil, fmt.Errorf("unsupported provider %q — only github is implemented", providerName)
			}
			if owner == "" || repo == "" {
				return nil, fmt.Errorf("invalid repo, expected %q", "owner/repo")
			}
			return provider.NewGitHubProvider(ghc, gqlAdapter, owner, repo, columnNames), nil
		}
		bp = provider.NewGitHubProvider(ghc, gqlAdapter, cfgOwner, cfgRepoName, columnNames)
	default:
		fmt.Fprintf(os.Stderr, "Unknown provider: %q\n", prov)
		os.Exit(1)
	}

	var watcher cenciwatch.Watcher
	if cfg.CenciValue() {
		watcher = cenciwatch.NewSocketWatcher()
	}

	// Ship built-in git actions, and a live git status reader, only inside a
	// git repo with a detected remote (a non-empty repo means push/pull have
	// somewhere to go, and status is meaningful).
	var defaultGitActions map[string]config.Action
	var gitReader gitdetect.Reader
	if gitInfo.Repo != "" {
		defaultGitActions = config.DefaultGitActions()
		gitReader = gitdetect.ExecReader{}
	}

	// Always-on crash reporting: a panic while BubbleTea owns the terminal
	// prints its stack to stderr, which the altscreen restore then wipes. The
	// deferred RecoverCrash guards in Update/View persist the stack to this
	// file before re-panicking. If the home dir can't be resolved, crash
	// logging simply stays disabled (empty path) — never block startup on it.
	if crashPath, err := config.DefaultCrashLogPath(); err == nil {
		debuglog.InitCrash(crashPath)
	}

	board := NewBoard(bp, defaultGitActions, cfg.Columns, action.DefaultExecutor{}, repoOwner, repoNameOnly, prov, cfg.SessionMaxLength, time.Duration(cfg.RefreshInterval)*time.Minute, cfg.WorkingLabelValue(), cfg.MouseValue(), false, watcher, gitReader, cfg.UpdateCheckValue())

	// Layer the loaded config's keymaps: over NewBoard's built-in defaults,
	// via config.ResolveKeymap -- the single resolution path every validator
	// already shares (internal/config/keymap_validate.go) and the only
	// source of the board's active keymap. withKeymap rebuilds every hint
	// bar immediately, so the very first rendered frame reflects the fully
	// resolved table, not just the defaults.
	km, err := config.ResolveKeymap(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving keymap: %v\n", err)
		os.Exit(1)
	}
	board = board.withKeymap(km)

	// Let a repo saved in the config modal retarget the board in place,
	// instead of writing the file and refreshing the previous repository.
	board.providerFactory = providerFactory
	// Scope the agents list to this instance's own tmux session (#410).
	board.tmuxSession = resolveTmuxSession(action.DefaultExecutor{})
	// trustPath lets in-app config Save() carry trust forward across a
	// rewrite of the local config file (#568, AC18).
	board.trustPath = trustPath
	// startupWarning is a one-shot hand-off: any untrusted-local-config strip
	// notices are surfaced as a timed status-bar warning on the first
	// successful board fetch, then cleared (handleBoardFetched, update.go).
	if len(cfg.Notices) > 0 {
		board.startupWarning = strings.Join(cfg.Notices, "; ")
	}
	// Seed the board-wide card sort direction: a previously toggled direction
	// (runtime state) wins over the configured default (#503). Cards are
	// fetched asynchronously, so no sort can run before this assignment.
	// A missing home dir or an unreadable/corrupt state file must not block
	// startup — log it and fall back to the configured default.
	statePath, err := config.DefaultStatePath()
	if err != nil {
		debuglog.Log(fmt.Sprintf("state: no state path available, sort order will not persist: %v", err))
	} else {
		board.statePath = statePath
	}
	var state config.State
	if board.statePath != "" {
		state, err = config.LoadState(board.statePath)
		if err != nil {
			debuglog.Log(fmt.Sprintf("state: ignoring unreadable state file: %v", err))
			state = config.State{}
		}
	}
	board.sortNewestFirst = config.ResolveSortNewestFirst(cfg, state)

	printNotices(os.Stderr, cfg.Deprecations, cfg.Notices)

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if cfg.MouseValue() {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(board, opts...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
