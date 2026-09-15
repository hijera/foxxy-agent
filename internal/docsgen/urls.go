package docsgen

// The documentation of this fork lives in the repository and nowhere else:
// there is no site layer in front of it (GitHub Pages serves docs/ only for
// config.schema.json), so a link that leaves the repository - the binary, the
// schema descriptions, the bundled skill, llms.txt - names the page on GitHub,
// and agents read the raw Markdown of the main branch.
const (
	GitHubBlob = "https://github.com/hijera/foxxy-agent/blob/main/"
	GitHubRaw  = "https://raw.githubusercontent.com/hijera/foxxy-agent/main/"
)

// PageURL is the address of a nav page for people: the file on GitHub.
func PageURL(navPath string) string { return GitHubBlob + RepoPath(navPath) }

// RawPageURL is the Markdown of a nav page as agents fetch it.
func RawPageURL(navPath string) string { return GitHubRaw + RepoPath(navPath) }
