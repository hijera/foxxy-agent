package shell

import "testing"

func TestValidateReadOnlyCommand(t *testing.T) {
	for _, command := range []string{
		"rg -n ModeAsk internal",
		"git status --short",
		"git diff -- internal/agent",
		"Get-ChildItem -Force",
		"Get-Content -LiteralPath README.md",
	} {
		t.Run("allow/"+command, func(t *testing.T) {
			if err := ValidateReadOnlyCommand(command); err != nil {
				t.Fatalf("expected read-only command to be allowed: %v", err)
			}
		})
	}

	for _, command := range []string{
		"Set-Content README.md changed",
		"Remove-Item README.md",
		"git checkout -- README.md",
		"git status; rm README.md",
		"rg token . | Set-Content result.txt",
		"rg token . > result.txt",
		"rg --pre ./filter token .",
		"Get-ChildItem\nRemove-Item README.md",
		"Write-Output ([IO.File]::WriteAllText('created.txt','changed'))",
		"./rg token .",
		`C:\tools\rg.exe token .`,
		"tree -o listing.txt",
		"date --set 2030-01-01",
		"go list ./...",
		"sort -InputObject value -Property {Set-Content created.txt changed}",
		"diff -ReferenceObject a -DifferenceObject b -Property {Remove-Item README.md}",
		"where -InputObject value -FilterScript {Set-Content created.txt changed}",
		"git ls-remote ext::touch created.txt",
		"git cat-file --filters HEAD:README.md",
		"git show --textconv HEAD:README.md",
	} {
		t.Run("deny/"+command, func(t *testing.T) {
			if err := ValidateReadOnlyCommand(command); err == nil {
				t.Fatal("expected mutating or compound command to be refused")
			}
		})
	}
}

// Subversion working copies get the same treatment as git: inspection
// subcommands are allowed, anything that writes or runs a helper program is not.
func TestValidateReadOnlySVNCommand(t *testing.T) {
	for _, command := range []string{
		"svn info",
		"svn status",
		"svn st --quiet",
		"svn log --limit 10",
		"svn diff -r 12:HEAD",
		"svn cat trunk/README.md",
		"svn list ^/branches",
		"svn blame src/main.go",
		"svn propget svn:externals",
	} {
		t.Run("allow/"+command, func(t *testing.T) {
			if err := ValidateReadOnlyCommand(command); err != nil {
				t.Fatalf("expected read-only svn command to be allowed: %v", err)
			}
		})
	}

	for _, command := range []string{
		"svn",
		"svn commit -m done",
		"svn update",
		"svn switch ^/branches/feature-x",
		"svn merge ^/trunk",
		"svn revert README.md",
		"svn checkout https://svn.example.test/repo/trunk wc",
		"svn delete README.md",
		"svn propset svn:externals value .",
		"svn diff --diff-cmd ./run.sh",
		"svn log --config-dir ./cfg",
		"svn status; rm README.md",
	} {
		t.Run("deny/"+command, func(t *testing.T) {
			if err := ValidateReadOnlyCommand(command); err == nil {
				t.Fatal("expected mutating or unsafe svn command to be refused")
			}
		})
	}
}
