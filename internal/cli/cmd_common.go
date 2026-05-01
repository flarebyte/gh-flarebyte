package cli

import (
	"fmt"
	"io"

	"github.com/flarebyte/gh-flarebyte/internal/config"
)

func loadConfigOrUsage(stderr io.Writer) (config.Config, *Result) {
	cfg, err := config.Load("")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		res := Result{ExitCode: ExitUsage, Err: err}
		return config.Config{}, &res
	}
	return cfg, nil
}

func resolveRepoOrConfigDefault(repo string, cfg config.Config) string {
	if repo != "" {
		return repo
	}
	return fmt.Sprintf("%s/%s", cfg.Project.Org, cfg.Project.Repo)
}

func loadRepoContext(repoArg string, stderr io.Writer) (string, config.Config, RepoMetadata, *Result) {
	cfg, usage := loadConfigOrUsage(stderr)
	if usage != nil {
		return "", config.Config{}, RepoMetadata{}, usage
	}
	repo := resolveRepoOrConfigDefault(repoArg, cfg)
	remote, err := readRepoMetadata(repo)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		res := Result{ExitCode: ExitFailure, Err: err}
		return "", config.Config{}, RepoMetadata{}, &res
	}
	return repo, cfg, remote, nil
}
