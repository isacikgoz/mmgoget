package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/google/go-github/v45/github"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	rg = regexp.MustCompile(`github\.com\/(.[^\/]+)/(.[^\/]+)/(v\d+)@(.+)`)

	commentFlag bool

	rootCmd = &cobra.Command{
		Use:   "mmgoget",
		Args:  cobra.MinimumNArgs(1),
		Short: "mmgoget is a command line tool to ease your pain while go getting a dependency",
		RunE:  RootCmdF,
	}
)

func main() {
	rootCmd.PersistentFlags().BoolVar(&commentFlag, "comment", false, "Places a comment above the require statement to show what project version the commit sha is pointing to.")

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

	if commentFlag {
		err := AddComment(args[0])
		if err != nil {
			return errors.Wrap(err, "error adding comment to go.mod")
		}
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

func AddComment(module string) error {
	parts := strings.Split(module, "@")
	if len(parts) != 2 {
		return errors.New("failed to parse version from module")
	}
	name, version := parts[0], parts[1]
	version = strings.TrimLeft(version, "v")

	b, err := os.ReadFile("go.mod")
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")

	out := []string{}
	for i, line := range lines {
		if !strings.Contains(line, name) {
			out = append(out, line)
			continue
		}

		if strings.HasPrefix(strings.Trim(out[i-1], "\t"), "//") {
			out = out[:i-1]
		}

		comment := "\t// " + version
		out = append(out, comment, line)
	}

	outBytes := []byte(strings.Join(out, "\n"))
	return os.WriteFile("go.mod", outBytes, 0777)
}
