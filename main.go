package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/google/go-github/v45/github"
	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
)

type Module struct {
	Owner   string
	Project string
	Version string
	Tag     string
	SHA     string
}

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

	mod, err := NewModule(args[0])
	if err != nil {
		return fmt.Errorf("failed to parse module: %v", err)
	}

	sha, err := GetSHA(mod)
	if err != nil {
		return fmt.Errorf("failed to get sha: %v", err)
	}
	mod.SHA = sha

	c := exec.Command("go", "get", fmt.Sprintf("%s@%s", mod.Path(), mod.SHA))
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		return fmt.Errorf("error while running command %q: %s", "go get", err)
	}

	if commentFlag {
		err := AddComment(mod)
		if err != nil {
			return fmt.Errorf("error adding comment to go.mod: %w", err)
		}
	}

	return nil
}

func GetSHA(mod *Module) (string, error) {
	client := github.NewClient(nil)

	opt := &github.ListOptions{
		PerPage: 200,
	}
	tags, _, err := client.Repositories.ListTags(context.Background(), mod.Owner, mod.Project, opt)
	if err != nil {
		return "", err
	}
	var sha string
	for _, t := range tags {
		if t.GetName() == mod.Tag {
			sha = t.GetCommit().GetSHA()
			sha = sha[:10]
			break
		}
	}
	if sha == "" {
		// leap of faith, maybe we can find with the tag
		// or the tag value is sha already
		sha = mod.Tag
	}

	return sha, nil
}

func AddComment(mod *Module) error {
	var b []byte
	var err error
	if b, err = ioutil.ReadFile("go.mod"); err != nil {
		return fmt.Errorf("failed to read go.mod: %v", err)
	}

	mf, err := modfile.Parse("go.mod", b, nil)
	if err != nil {
		return fmt.Errorf("failed to parse go.mod: %v", err)
	}

	for _, require := range mf.Require {
		if require.Mod.Path == mod.Path() {
			// cleanup previous mmgoget comments
			for i, c := range require.Syntax.Comments.Before {
				if strings.HasPrefix(c.Token, "// mmgoget") {
					require.Syntax.Comments.Before = append(require.Syntax.Comments.Before[:i], require.Syntax.Comments.Before[i+1:]...)
				}
			}

			require.Syntax.Comments.Before = append(require.Syntax.Comments.Before, modfile.Comment{
				Token: fmt.Sprintf("// mmgoget: %s@%s is replaced by -> %s@%s", mod.Path(), mod.Tag, mod.Path(), mod.SHA),
			})
		}
	}

	b1 := modfile.Format(mf.Syntax)
	err = os.Rename("go.mod", "go.mod.bak")
	if err != nil {
		return fmt.Errorf("failed to backup go.mod: %v", err)
	}
	if err = os.WriteFile("go.mod", b1, 0644); err != nil {
		_ = os.Rename("go.mod.bak", "go.mod")
		return err
	}

	defer os.Remove("./go.mod.bak")

	return nil
}

func NewModule(mod string) (*Module, error) {
	ss := rg.FindAllStringSubmatch(mod, -1)
	if len(ss) < 1 || len(ss[0]) < 5 {
		return nil, fmt.Errorf("format must be github.com/<org>/<repo>/<module_version>@<tag>")
	}

	owner, project, version, tag := ss[0][1], ss[0][2], ss[0][3], ss[0][4]

	return &Module{
		Owner:   owner,
		Project: project,
		Version: version,
		Tag:     tag,
	}, nil
}

func (m *Module) Path() string {
	return fmt.Sprintf("github.com/%s/%s/%s", m.Owner, m.Project, m.Version)
}
