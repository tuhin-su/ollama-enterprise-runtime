package manifest

import (
	"path/filepath"
	"testing"
)

func TestTotalTensorSize(t *testing.T) {
	m := &ModelManifest{
		Manifest: &Manifest{
			Layers: []ManifestLayer{
				{MediaType: "application/vnd.loom.image.tensor", Size: 1000},
				{MediaType: "application/vnd.loom.image.tensor", Size: 2000},
				{MediaType: "application/vnd.loom.image.json", Size: 500}, // not a tensor
				{MediaType: "application/vnd.loom.image.tensor", Size: 3000},
			},
		},
	}

	got := m.TotalTensorSize()
	want := int64(6000)
	if got != want {
		t.Errorf("TotalTensorSize() = %d, want %d", got, want)
	}
}

func TestTotalTensorSizeEmpty(t *testing.T) {
	m := &ModelManifest{
		Manifest: &Manifest{
			Layers: []ManifestLayer{},
		},
	}

	if got := m.TotalTensorSize(); got != 0 {
		t.Errorf("TotalTensorSize() = %d, want 0", got)
	}
}

func TestManifestAndBlobDirsRespectLOOMModels(t *testing.T) {
	modelsDir := filepath.Join(t.TempDir(), "models")

	// Simulate packaged/systemd environment
	t.Setenv("LOOM_MODELS", modelsDir)
	t.Setenv("HOME", "/usr/share/loom")

	// Manifest dir must respect LOOM_MODELS
	wantManifest := filepath.Join(modelsDir, "manifests")
	if got := DefaultManifestDir(); got != wantManifest {
		t.Fatalf("DefaultManifestDir() = %q, want %q", got, wantManifest)
	}

	// Blob dir must respect LOOM_MODELS
	wantBlobs := filepath.Join(modelsDir, "blobs")
	if got := DefaultBlobDir(); got != wantBlobs {
		t.Fatalf("DefaultBlobDir() = %q, want %q", got, wantBlobs)
	}
}
