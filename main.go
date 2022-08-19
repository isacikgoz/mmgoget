package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/google/go-github/v45/github"
	"github.com/spf13/cobra"
)

var (
	rg = regexp.MustCompile(`github\.com\/(.[^\/]+)/(.[^\/]+)/(v\d+)@(.+)`)

	rootCmd = &cobra.Command{
		Use:   "mmgoget",
		Args:  cobra.MinimumNArgs(1),
		Short: "mmgoget is a command line tool to ease your pain while go getting a dependency",
		RunE:  RootCmdF,
	}
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func RootCmdF(cmd *cobra.Command, args []string) error {
	if !rg.MatchString(args[0]) {
		return fmt.Errorf("format must be github.com/<org>/<repo>/<module_version>@<tag>")
	}

	ss := rg.FindAllStringSubmatch(args[0], -1)
	if len(ss) < 1 || len(ss[0]) < 5 {
		return fmt.Errorf("format must be github.com/<org>/<repo>/<module_version>@<tag>")
	}

	sha, err := GetSHA(ss[0][1], ss[0][2], ss[0][4])
	if err != nil {
		return fmt.Errorf("failed to get sha: %v", err)
	}

	c := exec.Command("go", "get", fmt.Sprintf("github.com/%s/%s/%s@%s", ss[0][1], ss[0][2], ss[0][3], sha))
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		return fmt.Errorf("error while running command %q: %s", "go get", err)
	}

	return nil
}

func GetSHA(owner, repo, tag string) (string, error) {
	client := github.NewClient(nil)

	opt := &github.ListOptions{
		PerPage: 200,
	}
	tags, _, err := client.Repositories.ListTags(context.Background(), owner, repo, opt)
	if err != nil {
		return "", err
	}
	var sha string
	for _, t := range tags {
		if t.GetName() == tag {
			sha = t.GetCommit().GetSHA()
			sha = sha[:10]
			break
		}
	}
	if sha == "" {
		// leap of faith, maybe we can find with the tag
		// or the tag value is sha already
		sha = tag
	}

	return sha, nil
}
