package commands

import (
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
)

func VersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Prints the version of migration-assist.",
		RunE:  versionCmdF,
	}
}

func versionCmdF(cmd *cobra.Command, _ []string) error {
	v, err := getVersionInfo()
	if err != nil {
		return err
	}

	cmd.Printf("migration-assist:\nVersion:\t%s\n"+
		"CommitDate:\t%s\nGitCommit:\t%s\nGitTreeState:\t%s\nGoVersion:\t%s\n"+
		"Compiler:\t%s\nPlatform:\t%s\n",
		Version, v.CommitDate, v.GitCommit, v.GitTreeState, v.GoVersion, v.Compiler, v.Platform)
	return nil
}

type Info struct {
	CommitDate   string
	GitCommit    string
	GitTreeState string
	GoVersion    string
	Compiler     string
	Platform     string
}

func getVersionInfo() (*Info, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, errors.New("failed to get build info")
	}

	var (
		revision     = "dev"
		gitTreeState = "dev"
		commitDate   = "dev"

		os       string
		arch     string
		compiler string
	)

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			commitDate = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				gitTreeState = "dirty"
			} else {
				gitTreeState = "clean"
			}
		case "GOOS":
			os = s.Value
		case "GOARCH":
			arch = s.Value
		case "-compiler":
			compiler = s.Value
		}
	}

	return &Info{
		CommitDate:   commitDate,
		GitCommit:    revision,
		GitTreeState: gitTreeState,
		GoVersion:    info.GoVersion,
		Compiler:     compiler,
		Platform:     fmt.Sprintf("%s/%s", arch, os),
	}, nil
}
