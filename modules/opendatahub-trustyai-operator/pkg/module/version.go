package module

import "fmt"

var (
	Version = "0.0.0-dev"
	Commit  = "unknown"
	Branch  = "unknown"
	Repo    = "unknown"
)

func BuildSource() string {
	return fmt.Sprintf("%s@%s/%s", Repo, Branch, Commit)
}
