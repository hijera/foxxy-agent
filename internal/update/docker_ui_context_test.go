// Tests that the image build can see every file the SPA bundler reaches for.
//
// The Dockerfile bundles the SPA in a Node stage whose context is external/ui and nothing else,
// so an import that climbs out of that tree - src/ui/auth/SignInScreen.tsx reads the two
// wordmarks straight out of docs/assets, on purpose - resolves to nothing unless the stage also
// copies that file in. Nothing builds the image before a release, so the first report was the
// release itself: "Docker build and push" failed on 0.2.94, 0.2.95, 0.3.0 and 0.3.1 with
// [UNRESOLVED_IMPORT] on those two wordmarks, and all four tags have a GitHub release but no
// container image. This test is the missing report - it lives here with the other checks on the
// release pipeline (the compose image, the tag bump, the release notes).
package update

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	// uiSourceTree is the only part of the repository the bundler stage is given.
	uiSourceTree = "external/ui"

	// uiBuildCommand is the RUN that bundles the SPA. Every COPY the bundler depends on has to
	// come before it in the same stage.
	uiBuildCommand = "npm run build:go"
)

// dockerRepoRoot is the repository root seen from this package.
var dockerRepoRoot = filepath.Join("..", "..")

// uiImport is one relative specifier of the SPA sources that lands outside external/ui.
type uiImport struct {
	source    string // the importing file, repository-relative, slash-separated
	specifier string // the specifier as written
	repoPath  string // what it resolves to in the repository
	imagePath string // what it resolves to inside the bundler stage
}

func TestDockerBuildContextHoldsWhatTheSPAImports(t *testing.T) {
	t.Parallel()
	stage := readUIBuilderStage(t)
	ignore := readDockerignore(t)

	for _, ref := range spaImportsOutsideTheUITree(t, stage.uiRoot) {
		if _, err := os.Stat(filepath.Join(dockerRepoRoot, filepath.FromSlash(ref.repoPath))); err != nil {
			t.Errorf("%s imports %q, which is no file in this repository (%s)", ref.source, ref.specifier, ref.repoPath)
			continue
		}
		if !stage.places(ref.repoPath, ref.imagePath) {
			t.Errorf("%s imports %q, which the bundler resolves to %s inside the %q stage of the Dockerfile, "+
				"but no COPY before `%s` puts %s there: the image build stops at [UNRESOLVED_IMPORT] and the "+
				"release publishes no image.\nAdd `COPY %s %s/` to that stage, or import an asset that lives "+
				"inside %s.",
				ref.source, ref.specifier, ref.imagePath, stage.name, uiBuildCommand, ref.repoPath,
				ref.repoPath, path.Dir(ref.imagePath), uiSourceTree)
		}
		if pattern, ok := ignore.excludes(ref.repoPath); ok {
			t.Errorf("%s imports %s, which .dockerignore keeps out of the build context (pattern %q), so the "+
				"COPY in the %q stage fails before the bundler is reached",
				ref.source, ref.repoPath, pattern, stage.name)
		}
	}
}

// A scan that matches nothing would pass the check above forever. The imports it is written for
// are known, so it says so: finding them is the proof that the walk still reaches the sources and
// that the pattern still reads an import.
func TestSPAImportsOutsideTheUITreeAreFound(t *testing.T) {
	t.Parallel()
	found := map[string][]string{}
	for _, ref := range spaImportsOutsideTheUITree(t, readUIBuilderStage(t).uiRoot) {
		found[ref.source] = append(found[ref.source], ref.repoPath)
	}
	const signIn = uiSourceTree + "/src/ui/auth/SignInScreen.tsx"
	for _, want := range []string{
		"docs/assets/foxxycode-logo-wordmark.svg",
		"docs/assets/foxxycode-logo-wordmark-light.svg",
	} {
		if !slices.Contains(found[signIn], want) {
			t.Fatalf("the scan of %s/src did not find the %s import of %s; it found %v", uiSourceTree, want, signIn, found)
		}
	}
}

// uiOutsideImportPattern matches a relative specifier that climbs at least one directory:
// `from "../x"`, `import "../x"`, `import("../x")`, `require("../x")` and CSS `url("../x")`.
var uiOutsideImportPattern = regexp.MustCompile(`(?:\bfrom|\bimport|\brequire|\burl)\s*\(?\s*["']((?:\.\./)+[^"'\n]+)["']`)

// uiOutsideURLPattern matches the unquoted CSS form, `url(../x)`.
var uiOutsideURLPattern = regexp.MustCompile(`\burl\(\s*((?:\.\./)+[^)'"\s]+)\s*\)`)

// spaImportsOutsideTheUITree returns every specifier in the bundled SPA sources that resolves
// outside external/ui, resolved both in the repository and under uiRoot, the stage's copy of
// external/ui. Test files are skipped: vitest runs them in a checkout, never in the image.
func spaImportsOutsideTheUITree(t *testing.T, uiRoot string) []uiImport {
	t.Helper()
	srcRoot := filepath.Join(dockerRepoRoot, filepath.FromSlash(uiSourceTree), "src")
	var refs []uiImport
	err := filepath.WalkDir(srcRoot, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		base := entry.Name()
		switch filepath.Ext(base) {
		case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".css":
		default:
			return nil
		}
		if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
			return nil
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dockerRepoRoot, name)
		if err != nil {
			return err
		}
		source := filepath.ToSlash(rel)
		for _, pattern := range []*regexp.Regexp{uiOutsideImportPattern, uiOutsideURLPattern} {
			for _, match := range pattern.FindAllStringSubmatch(string(raw), -1) {
				// `?raw`, `?url` and a fragment are Vite's, not part of the path.
				specifier, _, _ := strings.Cut(match[1], "?")
				specifier, _, _ = strings.Cut(specifier, "#")
				repoPath := path.Join(path.Dir(source), specifier)
				if strings.HasPrefix(repoPath, uiSourceTree+"/") {
					continue
				}
				refs = append(refs, uiImport{
					source:    source,
					specifier: match[1],
					repoPath:  repoPath,
					// Everything above the stage's copy of external/ui collapses onto its root,
					// which is how `../../../../../docs/assets/x` written in src/ui/auth arrives
					// at /docs/assets/x with external/ui unpacked at /ui.
					imagePath: path.Join(uiRoot, path.Dir(strings.TrimPrefix(source, uiSourceTree+"/")), specifier),
				})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", srcRoot, err)
	}
	return refs
}

// dockerStage is the bundler stage of the Dockerfile: its name, its working directory, where it
// unpacks external/ui and the COPY instructions that run before the SPA is bundled.
type dockerStage struct {
	name    string
	workdir string
	uiRoot  string
	copies  []dockerCopy
}

type dockerCopy struct {
	sources []string
	dest    string
}

// places reports whether a COPY of the stage puts the repository file at the given image path.
func (s dockerStage) places(repoPath, imagePath string) bool {
	for _, instruction := range s.copies {
		dest := s.resolve(instruction.dest)
		for _, source := range instruction.sources {
			matched, err := path.Match(source, repoPath)
			if err != nil || (!matched && source != repoPath) {
				continue
			}
			// A single source with a destination that is not a directory is a rename; anything
			// else keeps the base name inside the destination directory.
			placed := path.Join(dest, path.Base(repoPath))
			if len(instruction.sources) == 1 && source == repoPath && !strings.HasSuffix(instruction.dest, "/") {
				placed = dest
			}
			if placed == imagePath {
				return true
			}
		}
	}
	return false
}

// resolve turns a COPY destination into an absolute path inside the image.
func (s dockerStage) resolve(dest string) string {
	if path.IsAbs(dest) {
		return path.Clean(dest)
	}
	return path.Join(s.workdir, dest)
}

// readUIBuilderStage parses the Dockerfile down to the stage that bundles the SPA, keeping the
// COPY instructions that precede the bundling RUN and where that stage holds external/ui.
func readUIBuilderStage(t *testing.T) dockerStage {
	t.Helper()
	dockerfile := filepath.Join(dockerRepoRoot, "Dockerfile")
	raw, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatalf("read %s: %v", dockerfile, err)
	}

	var stage, current dockerStage
	inStage := false
	for _, instruction := range dockerInstructions(string(raw)) {
		verb, rest, _ := strings.Cut(instruction, " ")
		switch strings.ToUpper(verb) {
		case "FROM":
			fields := strings.Fields(rest)
			current = dockerStage{workdir: "/"}
			if len(fields) >= 3 && strings.EqualFold(fields[len(fields)-2], "AS") {
				current.name = fields[len(fields)-1]
			}
			inStage = true
		case "WORKDIR":
			if fields := dockerArguments(rest); inStage && len(fields) == 1 {
				current.workdir = current.resolve(fields[0])
			}
		case "COPY":
			fields := dockerArguments(rest)
			if !inStage || len(fields) < 2 {
				continue
			}
			current.copies = append(current.copies, dockerCopy{sources: fields[:len(fields)-1], dest: fields[len(fields)-1]})
		case "RUN":
			if inStage && strings.Contains(rest, uiBuildCommand) {
				stage = current
				// Whatever the stage copies after the bundle is built is no use to the bundler.
				inStage = false
			}
		}
	}
	if stage.name == "" {
		t.Fatalf("no named stage of %s runs %q; this test no longer knows where the SPA is bundled", dockerfile, uiBuildCommand)
	}

	for _, instruction := range stage.copies {
		for _, source := range instruction.sources {
			if strings.TrimSuffix(source, "/") == uiSourceTree {
				stage.uiRoot = stage.resolve(instruction.dest)
			}
		}
	}
	if stage.uiRoot == "" {
		t.Fatalf("the %q stage of %s does not COPY %s; this test no longer knows where the SPA sources land",
			stage.name, dockerfile, uiSourceTree)
	}
	return stage
}

// dockerInstructions splits a Dockerfile into instructions, joining continuation lines and
// dropping comments.
func dockerInstructions(dockerfile string) []string {
	var instructions []string
	pending := ""
	for _, line := range strings.Split(strings.ReplaceAll(dockerfile, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasSuffix(trimmed, "\\") {
			pending += strings.TrimSpace(strings.TrimSuffix(trimmed, "\\")) + " "
			continue
		}
		if instruction := strings.TrimSpace(pending + trimmed); instruction != "" {
			instructions = append(instructions, instruction)
		}
		pending = ""
	}
	return instructions
}

// dockerArguments returns the arguments of an instruction without its --flags, unwrapping the
// JSON array form (["a", "b"]).
func dockerArguments(rest string) []string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "[") {
		rest = strings.NewReplacer("[", " ", "]", " ", ",", " ").Replace(rest)
	}
	var arguments []string
	for _, field := range strings.Fields(rest) {
		if strings.HasPrefix(field, "--") {
			continue
		}
		arguments = append(arguments, strings.Trim(field, `"`))
	}
	return arguments
}

// dockerignoreFile is the exclusion list of the build context.
type dockerignoreFile []string

// excludes reports whether a pattern keeps the path out of the build context, and which one. The
// match is deliberately coarse - one `**/` prefix, `!` re-inclusion ignored - because all it has
// to notice is a pattern that would take an imported asset away, and such a pattern breaks the
// image build anyway.
func (f dockerignoreFile) excludes(repoPath string) (string, bool) {
	segments := strings.Split(repoPath, "/")
	for _, pattern := range f {
		clean := strings.Trim(pattern, "/")
		suffix, anywhere := strings.CutPrefix(clean, "**/")
		// A pattern that excludes a directory excludes everything under it, so every prefix of
		// the path is a candidate.
		for i := 1; i <= len(segments); i++ {
			if matched, _ := path.Match(clean, strings.Join(segments[:i], "/")); matched {
				return pattern, true
			}
			if !anywhere {
				continue
			}
			for j := range i {
				if matched, _ := path.Match(suffix, strings.Join(segments[j:i], "/")); matched {
					return pattern, true
				}
			}
		}
	}
	return "", false
}

func readDockerignore(t *testing.T) dockerignoreFile {
	t.Helper()
	name := filepath.Join(dockerRepoRoot, ".dockerignore")
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var patterns dockerignoreFile
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		patterns = append(patterns, trimmed)
	}
	return patterns
}
