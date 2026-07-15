package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
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

func main() {
	if versionRequested(os.Args) {
		fmt.Printf("lazyboards %s\n", appVersion())
		return
	}

	globalPath, err := config.DefaultGlobalPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving config path: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(globalPath, config.DefaultLocalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Auto-detect provider and repo from git remote
	gitInfo := gitdetect.DetectRemote(".git/config")

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
		board := NewBoard(nil, nil, nil, nil, nil, repoOwner, repoNameOnly, prov, 0, 0, 0, config.DefaultWorkingLabel, false, true, nil, nil)
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
		// Reload config with saved values
		cfg, err = config.Load(globalPath, config.DefaultLocalPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
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
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
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
		bp = provider.NewGitHubProvider(ghc, gqlAdapter, parts[0], parts[1], cfg.ColumnNames())
	default:
		fmt.Fprintf(os.Stderr, "Unknown provider: %q\n", prov)
		os.Exit(1)
	}

	var watcher cenciwatch.Watcher
	if cfg.AgentWatchValue() {
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

	if err := debuglog.Init(os.Getenv("LAZYBOARDS_DEBUG_LOG")); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening debug log: %v\n", err)
	}

	board := NewBoard(bp, cfg.Actions, defaultGitActions, cfg.Columns, action.DefaultExecutor{}, repoOwner, repoNameOnly, prov, cfg.SessionMaxLength, time.Duration(cfg.RefreshInterval)*time.Minute, time.Duration(cfg.ActionRefreshDelayValue())*time.Second, cfg.WorkingLabelValue(), cfg.MouseValue(), false, watcher, gitReader)

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
