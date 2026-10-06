package shell

import "testing"

// TestLongRunningCommandDetection covers the commands that never exit on their
// own. Giving them a hard timeout is always wrong: the process is started so that
// later steps can talk to it, so killing it on a clock breaks the very thing it
// was started for. Getting a false positive merely means the task runs until it
// ends or is stopped; a false negative kills a dev server, so the list leans on
// unmistakable invocations only.
func TestLongRunningCommandDetection(t *testing.T) {
	longRunning := []string{
		"yarn serve --port 8082",
		"yarn serve",
		"npm run dev",
		"npm start",
		"pnpm dev",
		"bun run dev",
		"npx vite",
		"vite preview",
		"next dev",
		"nodemon index.js",
		"webpack serve",
		"vue-cli-service serve",
		"php artisan serve",
		"rails server",
		"python -m http.server 8000",
		"go run ./cmd/server --watch",
		"tsc --watch",
		"docker compose up",
		// Shell noise around the real command must not hide it.
		"cd frontend && yarn serve --port 3000",
	}
	for _, cmd := range longRunning {
		t.Run(cmd, func(t *testing.T) {
			if !isLongRunningCommand(cmd) {
				t.Errorf("isLongRunningCommand(%q) = false, want true", cmd)
			}
		})
	}

	terminating := []string{
		"npm run build",
		"npm test",
		"yarn build",
		"yarn install",
		"go build ./...",
		"go test ./...",
		"make test",
		"git status",
		"docker compose up -d",
		"docker compose down",
		"ls -la",
		"npx tsc",
		"python script.py",
		// "serve" as a noun in a path is not the serve subcommand.
		"cat src/serve/readme.md",
		"",
	}
	for _, cmd := range terminating {
		t.Run("terminating/"+cmd, func(t *testing.T) {
			if isLongRunningCommand(cmd) {
				t.Errorf("isLongRunningCommand(%q) = true, want false", cmd)
			}
		})
	}
}
