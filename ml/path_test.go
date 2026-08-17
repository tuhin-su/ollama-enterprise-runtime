package ml

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLibLoomPath(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name   string
		search libLoomPathSearch
		dirs   []string
		want   string
	}{
		{
			name: "darwin release layout",
			search: libLoomPathSearch{
				executable: filepath.Join(root, "darwin-app", "Loom.app", "Contents", "Resources", "loom"),
				goos:       "darwin",
				goarch:     "arm64",
			},
			dirs: []string{filepath.Join(root, "darwin-app", "Loom.app", "Contents", "Resources")},
			want: filepath.Join(root, "darwin-app", "Loom.app", "Contents", "Resources"),
		},
		{
			name: "darwin standard install layout",
			search: libLoomPathSearch{
				executable: filepath.Join(root, "darwin-install", "bin", "loom"),
				goos:       "darwin",
				goarch:     "arm64",
			},
			dirs: []string{filepath.Join(root, "darwin-install", "lib", "loom")},
			want: filepath.Join(root, "darwin-install", "lib", "loom"),
		},
		{
			name: "windows release layout",
			search: libLoomPathSearch{
				executable: filepath.Join(root, "windows-release", "loom.exe"),
				goos:       "windows",
				goarch:     "amd64",
			},
			dirs: []string{filepath.Join(root, "windows-release", "lib", "loom")},
			want: filepath.Join(root, "windows-release", "lib", "loom"),
		},
		{
			name: "windows standard install layout",
			search: libLoomPathSearch{
				executable: filepath.Join(root, "windows-install", "bin", "loom.exe"),
				goos:       "windows",
				goarch:     "amd64",
			},
			dirs: []string{filepath.Join(root, "windows-install", "lib", "loom")},
			want: filepath.Join(root, "windows-install", "lib", "loom"),
		},
		{
			name: "linux standard install layout",
			search: libLoomPathSearch{
				executable: filepath.Join(root, "linux-install", "bin", "loom"),
				goos:       "linux",
				goarch:     "amd64",
			},
			dirs: []string{filepath.Join(root, "linux-install", "lib", "loom")},
			want: filepath.Join(root, "linux-install", "lib", "loom"),
		},
		{
			name: "local linux underscore dist layout",
			search: libLoomPathSearch{
				executable: filepath.Join(root, "linux-dev", "loom"),
				workingDir: filepath.Join(root, "linux-dev"),
				goos:       "linux",
				goarch:     "amd64",
			},
			dirs: []string{filepath.Join(root, "linux-dev", "dist", "linux_amd64", "lib", "loom")},
			want: filepath.Join(root, "linux-dev", "dist", "linux_amd64", "lib", "loom"),
		},
		{
			name: "mlx-only standard install layout",
			search: libLoomPathSearch{
				executable: filepath.Join(root, "mlx-install", "bin", "loom"),
				goos:       "linux",
				goarch:     "amd64",
			},
			dirs: []string{filepath.Join(root, "mlx-install", "lib", "loom")},
			want: filepath.Join(root, "mlx-install", "lib", "loom"),
		},
		{
			name: "darwin local build layout before executable directory fallback",
			search: libLoomPathSearch{
				executable: filepath.Join(root, "darwin-dev", "loom"),
				workingDir: filepath.Join(root, "darwin-dev"),
				goos:       "darwin",
				goarch:     "arm64",
			},
			dirs: []string{filepath.Join(root, "darwin-dev", "build", "lib", "loom")},
			want: filepath.Join(root, "darwin-dev", "build", "lib", "loom"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, dir := range tt.dirs {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			got := findLibLoomPath(tt.search)
			if got != tt.want {
				t.Fatalf("findLibLoomPath() = %q, want %q; candidates: %v", got, tt.want, libLoomPathCandidates(tt.search))
			}
		})
	}
}
