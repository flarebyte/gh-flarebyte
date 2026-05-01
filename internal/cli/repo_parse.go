package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func parseRepoInitArgs(args []string) (repo string, overwrite bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--overwrite":
			overwrite = true
		case "--repo":
			if i+1 >= len(args) {
				return "", false, errors.New("invalid invocation: --repo requires owner/name")
			}
			repo = args[i+1]
			i++
		default:
			return "", false, fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return repo, overwrite, nil
}

func parseRepoAuditArgs(args []string) (repo string, asJSON bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--repo":
			if i+1 >= len(args) {
				return "", false, errors.New("invalid invocation: --repo requires owner/name")
			}
			repo = args[i+1]
			i++
		case "--json":
			asJSON = true
		default:
			return "", false, fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return repo, asJSON, nil
}

func parseRepoUpdateArgs(args []string) (repo string, confirmDeletions bool, acceptVisibility bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--repo":
			if i+1 >= len(args) {
				return "", false, false, errors.New("invalid invocation: --repo requires owner/name")
			}
			repo = args[i+1]
			i++
		case "--confirm-deletions":
			confirmDeletions = true
		case "--accept-visibility-change-consequences":
			acceptVisibility = true
		default:
			return "", false, false, fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return repo, confirmDeletions, acceptVisibility, nil
}

func parseReposMineArgs(args []string) (org string, asJSON bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--org":
			if i+1 >= len(args) {
				return "", false, errors.New("invalid invocation: --org requires a value")
			}
			org = args[i+1]
			i++
		case "--json":
			asJSON = true
		default:
			return "", false, fmt.Errorf("invalid invocation: unknown argument %q", arg)
		}
	}
	return org, asJSON, nil
}

func splitRepo(repo string) (owner string, name string, err error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid invocation: --repo must use owner/name")
	}
	return parts[0], parts[1], nil
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
