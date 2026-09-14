// Command updateplugins turns a built IntelliJ plugin zip into the updatePlugins.xml document
// that JetBrains IDEs poll for plugin updates.
//
//	go run ./editors/intellij/updateplugins \
//	  -zip editors/intellij/build/distributions/foxxycode-intellij-1.2.3.zip \
//	  -tag 1.2.3 \
//	  -out docs/updatePlugins.xml
//
// Run by .github/workflows/intellij-plugin.yaml on every release tag, and by
// .github/workflows/plugin-repository.yaml when a version is pinned or rolled back by hand.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "updateplugins:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("updateplugins", flag.ContinueOnError)
	var (
		archivePath = flags.String("zip", "", "path to the built plugin zip (required)")
		tag         = flags.String("tag", "", "release tag the zip is published under (required)")
		repository  = flags.String("repo", "hijera/foxxy-agent", "GitHub repository owner/name hosting the release")
		outPath     = flags.String("out", "docs/updatePlugins.xml", "repository document to write")
		pluginID    = flags.String("plugin-id", "dev.foxxycode.intellij", "plugin id the zip must declare")
		pluginName  = flags.String("plugin-name", "FoxxyCode", "plugin name the zip must declare")
		force       = flags.Bool("force", false, "publish even when the document already advertises this version or a newer one")
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *archivePath == "" || *tag == "" {
		return fmt.Errorf("-zip and -tag are required")
	}

	found, err := inspect(*archivePath, *pluginID)
	if err != nil {
		return err
	}
	if err := validate(found, *pluginID, *pluginName, *tag); err != nil {
		return err
	}

	// Without this a workflow_dispatch rebuild of an older tag would point every IDE
	// backwards. -force is how a deliberate rollback says it means it.
	if !*force {
		current, err := publishedVersion(*outPath)
		if err != nil {
			return err
		}
		if current != "" && compareVersions(found.Version, current) <= 0 {
			_, _ = fmt.Fprintf(out, "%s already advertises %s; leaving it alone (pass -force to override).\n", *outPath, current)
			return nil
		}
	}

	document, err := render(found, downloadURL(*repository, *tag, filepath.Base(*archivePath)))
	if err != nil {
		return err
	}
	if err := writeDocument(*outPath, document); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(out, "%s now advertises %s %s (since-build %s) from %s.\n",
		*outPath, found.Name, found.Version, found.SinceBuild, *repository)
	return nil
}
