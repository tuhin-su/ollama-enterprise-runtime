package ml

import (
	"os"
	"path/filepath"
	"runtime"
)

type libLoomPathSearch struct {
	executable string
	workingDir string
	goos       string
	goarch     string
}

// LibLoomPath is the root used to find bundled llama.cpp and MLX runtime
// libraries. GPU-specific libraries live in backend subdirectories such as
// cuda_v12, rocm_v7_2, vulkan, and mlx_cuda_v13.
var LibLoomPath = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if eval, err := filepath.EvalSymlinks(exe); err == nil {
		exe = eval
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	return findLibLoomPath(libLoomPathSearch{
		executable: exe,
		workingDir: cwd,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
	})
}()

func findLibLoomPath(search libLoomPathSearch) string {
	candidates := libLoomPathCandidates(search)
	for _, path := range candidates {
		if libLoomPathExists(path) {
			return path
		}
	}

	if search.executable != "" {
		return filepath.Dir(search.executable)
	}
	return ""
}

func libLoomPathCandidates(search libLoomPathSearch) []string {
	goos := search.goos
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := search.goarch
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	seen := map[string]bool{}
	var candidates []string
	add := func(path string) {
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			candidates = append(candidates, path)
		}
	}

	if search.executable != "" {
		exeDir := filepath.Dir(search.executable)
		switch goos {
		case "darwin":
			// Local dist output and standard installs keep helpers under lib/loom.
			add(filepath.Join(exeDir, "lib", "loom"))
			add(filepath.Join(exeDir, "..", "lib", "loom"))
		case "linux":
			add(filepath.Join(exeDir, "..", "lib", "loom"))
			add(filepath.Join(exeDir, "lib", "loom"))
		case "windows":
			add(filepath.Join(exeDir, "lib", "loom"))
			add(filepath.Join(exeDir, "..", "lib", "loom"))
		default:
			add(filepath.Join(exeDir, "lib", "loom"))
			add(filepath.Join(exeDir, "..", "lib", "loom"))
		}
		addLocalLibLoomPaths(add, exeDir, goos, goarch)
		if goos == "darwin" {
			// macOS release artifacts colocate native helpers with loom.
			add(exeDir)
		}
	}
	addLocalLibLoomPaths(add, search.workingDir, goos, goarch)

	return candidates
}

func addLocalLibLoomPaths(add func(string), base, goos, goarch string) {
	if base == "" {
		return
	}
	add(filepath.Join(base, "build", "lib", "loom"))
	add(filepath.Join(base, "dist", goos+"-"+goarch, "lib", "loom"))
	if goos+"_"+goarch != goos+"-"+goarch {
		add(filepath.Join(base, "dist", goos+"_"+goarch, "lib", "loom"))
	}
	if goos == "darwin" {
		add(filepath.Join(base, "dist", "darwin"))
	}
}

func libLoomPathExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
