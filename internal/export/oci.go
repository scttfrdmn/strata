// Package export converts Strata lockfiles to portable container formats.
//
// The primary export format is OCI Image Layout (OCI Image Layout Specification
// 1.0), which is compatible with Docker, Podman, and Apptainer.
package export

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scttfrdmn/strata/internal/overlay"
	"github.com/scttfrdmn/strata/spec"
)

// Export converts a lockfile's layers into an OCI Image Layout tar archive.
// Requires unsquashfs (squashfs-tools) to be installed on PATH.
//
// layerPaths must be ordered: the first entry is the bottom-most layer
// (lowest MountOrder). Layers are combined into separate OCI diff layers.
// The resulting archive at outputPath is a valid OCI Image Layout that can
// be loaded with:
//
//	docker load -i outputPath
//	podman load -i outputPath
func Export(ctx context.Context, lf *spec.LockFile, layerPaths []overlay.LayerPath, outputPath, tag string) error {
	if tag == "" {
		tag = "strata:latest"
	}

	// Sort layerPaths by MountOrder ascending (bottom of stack first).
	sorted := make([]overlay.LayerPath, len(layerPaths))
	copy(sorted, layerPaths)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].MountOrder < sorted[j].MountOrder
	})

	workDir, err := os.MkdirTemp("", "strata-oci-*")
	if err != nil {
		return fmt.Errorf("oci: creating work dir: %w", err)
	}
	defer os.RemoveAll(workDir) //nolint:errcheck

	// Unpack each squashfs into its own directory, tar it up, compute digests.
	var layers []ociLayer
	var diffIDs []string
	for _, lp := range sorted {
		unpackDir := filepath.Join(workDir, "layer-"+lp.ID)
		if err := unpackSquashfs(ctx, lp.Path, unpackDir); err != nil {
			return fmt.Errorf("oci: unpacking layer %q: %w", lp.ID, err)
		}
		tarBytes, digest, err := tarDirDeterministic(unpackDir)
		if err != nil {
			return fmt.Errorf("oci: taring layer %q: %w", lp.ID, err)
		}
		diffIDs = append(diffIDs, "sha256:"+digest)
		layers = append(layers, ociLayer{
			DiffID: "sha256:" + digest,
			Digest: "sha256:" + digest,
			Size:   int64(len(tarBytes)),
			Blob:   tarBytes,
		})
	}

	// Build OCI image config.
	envVars := buildOCIEnv(lf)
	labels := buildOCILabels(lf, layerPaths)
	cfg := ociImageConfig{
		Env:    envVars,
		Cmd:    []string{"/bin/sh"},
		Labels: labels,
	}

	return writeOCILayout(cfg, layers, diffIDs, tag, outputPath)
}

// unpackSquashfs unpacks a .sqfs file into dir using unsquashfs.
func unpackSquashfs(ctx context.Context, sqfsPath, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// unsquashfs requires the target directory to not exist; use -d flag.
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "unsquashfs", "-d", dir, sqfsPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unsquashfs: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}

// TarDirDeterministic creates a deterministic tar archive of dir.
// Files are sorted by path, timestamps are zeroed, and uid/gid are cleared.
// Returns the raw tar bytes and their sha256 hex digest.
func TarDirDeterministic(dir string) ([]byte, string, error) {
	return tarDirDeterministic(dir)
}

func tarDirDeterministic(dir string) ([]byte, string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("walking dir: %w", err)
	}
	sort.Strings(paths)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	epoch := time.Unix(0, 0)

	for _, rel := range paths {
		fullPath := filepath.Join(dir, rel)
		fi, err := os.Lstat(fullPath)
		if err != nil {
			return nil, "", err
		}

		// Use tar.FileInfoHeader for correct type mapping, then zero
		// out non-deterministic fields (timestamp, ownership).
		var linkTarget string
		if fi.Mode()&fs.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(fullPath)
			if err != nil {
				return nil, "", err
			}
		}
		hdr, err := tar.FileInfoHeader(fi, linkTarget)
		if err != nil {
			return nil, "", err
		}
		hdr.Name = rel
		hdr.ModTime = epoch
		hdr.AccessTime = epoch
		hdr.ChangeTime = epoch
		hdr.Uid = 0
		hdr.Gid = 0
		hdr.Uname = ""
		hdr.Gname = ""
		hdr.Format = tar.FormatPAX

		if err := tw.WriteHeader(hdr); err != nil {
			return nil, "", err
		}

		if fi.Mode().IsRegular() {
			f, err := os.Open(fullPath)
			if err != nil {
				return nil, "", err
			}
			if _, err := io.Copy(tw, f); err != nil {
				f.Close() //nolint:errcheck
				return nil, "", err
			}
			f.Close() //nolint:errcheck
		}
	}

	if err := tw.Close(); err != nil {
		return nil, "", err
	}

	tarBytes := buf.Bytes()
	sum := sha256.Sum256(tarBytes)
	return tarBytes, hex.EncodeToString(sum[:]), nil
}

// ociImageConfig carries the OCI image configuration metadata.
type ociImageConfig struct {
	Env    []string
	Cmd    []string
	Labels map[string]string
}

// ociLayer holds one OCI diff layer blob and its metadata.
type ociLayer struct {
	DiffID string
	Digest string
	Size   int64
	Blob   []byte
}

// writeOCILayout writes the full OCI Image Layout to outputPath (.tar).
func writeOCILayout(cfg ociImageConfig, layers []ociLayer, diffIDs []string, tag, outputPath string) error {
	// Build OCI image config JSON.
	type rootFSConfig struct {
		Type    string   `json:"type"`
		DiffIDs []string `json:"diff_ids"`
	}
	type imageConfig struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
		Config       struct {
			Env    []string          `json:"Env,omitempty"`
			Cmd    []string          `json:"Cmd,omitempty"`
			Labels map[string]string `json:"Labels,omitempty"`
		} `json:"config"`
		RootFS rootFSConfig `json:"rootfs"`
	}

	var ic imageConfig
	ic.Architecture = "amd64"
	ic.OS = "linux"
	ic.Config.Env = cfg.Env
	ic.Config.Cmd = cfg.Cmd
	ic.Config.Labels = cfg.Labels
	ic.RootFS = rootFSConfig{Type: "layers", DiffIDs: diffIDs}

	configJSON, err := json.Marshal(ic)
	if err != nil {
		return fmt.Errorf("oci: marshaling image config: %w", err)
	}
	configSum := sha256.Sum256(configJSON)
	configDigest := "sha256:" + hex.EncodeToString(configSum[:])

	// Build manifest.
	type descriptor struct {
		MediaType   string            `json:"mediaType"`
		Digest      string            `json:"digest"`
		Size        int64             `json:"size"`
		Annotations map[string]string `json:"annotations,omitempty"`
	}
	type manifest struct {
		SchemaVersion int          `json:"schemaVersion"`
		MediaType     string       `json:"mediaType"`
		Config        descriptor   `json:"config"`
		Layers        []descriptor `json:"layers"`
	}

	var mfst manifest
	mfst.SchemaVersion = 2
	mfst.MediaType = "application/vnd.oci.image.manifest.v1+json"
	mfst.Config = descriptor{
		MediaType: "application/vnd.oci.image.config.v1+json",
		Digest:    configDigest,
		Size:      int64(len(configJSON)),
	}
	for _, l := range layers {
		mfst.Layers = append(mfst.Layers, descriptor{
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Digest:    l.Digest,
			Size:      l.Size,
		})
	}

	mfstJSON, err := json.Marshal(mfst)
	if err != nil {
		return fmt.Errorf("oci: marshaling manifest: %w", err)
	}
	mfstSum := sha256.Sum256(mfstJSON)
	mfstDigest := "sha256:" + hex.EncodeToString(mfstSum[:])

	// Build index.json.
	type index struct {
		SchemaVersion int          `json:"schemaVersion"`
		MediaType     string       `json:"mediaType"`
		Manifests     []descriptor `json:"manifests"`
	}
	idx := index{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests: []descriptor{{
			MediaType:   "application/vnd.oci.image.manifest.v1+json",
			Digest:      mfstDigest,
			Size:        int64(len(mfstJSON)),
			Annotations: map[string]string{"org.opencontainers.image.ref.name": tag},
		}},
	}
	idxJSON, err := json.Marshal(idx)
	if err != nil {
		return fmt.Errorf("oci: marshaling index: %w", err)
	}

	// Write the OCI Image Layout tar archive.
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("oci: creating output file: %w", err)
	}
	tw := tar.NewWriter(out)

	writeBlob := func(name string, data []byte) error {
		hdr := &tar.Header{
			Name:    name,
			Size:    int64(len(data)),
			Mode:    0644,
			ModTime: time.Unix(0, 0),
			Format:  tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}

	// oci-layout
	ociLayout := []byte(`{"imageLayoutVersion":"1.0.0"}`)
	if err := writeBlob("oci-layout", ociLayout); err != nil {
		out.Close() //nolint:errcheck
		return fmt.Errorf("oci: writing oci-layout: %w", err)
	}
	// index.json
	if err := writeBlob("index.json", idxJSON); err != nil {
		out.Close() //nolint:errcheck
		return fmt.Errorf("oci: writing index.json: %w", err)
	}
	// blobs/sha256/<config>
	configPath := "blobs/sha256/" + strings.TrimPrefix(configDigest, "sha256:")
	if err := writeBlob(configPath, configJSON); err != nil {
		out.Close() //nolint:errcheck
		return fmt.Errorf("oci: writing config blob: %w", err)
	}
	// blobs/sha256/<manifest>
	mfstPath := "blobs/sha256/" + strings.TrimPrefix(mfstDigest, "sha256:")
	if err := writeBlob(mfstPath, mfstJSON); err != nil {
		out.Close() //nolint:errcheck
		return fmt.Errorf("oci: writing manifest blob: %w", err)
	}
	// layer blobs
	for _, l := range layers {
		blobPath := "blobs/sha256/" + strings.TrimPrefix(l.Digest, "sha256:")
		if err := writeBlob(blobPath, l.Blob); err != nil {
			out.Close() //nolint:errcheck
			return fmt.Errorf("oci: writing layer blob: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		out.Close() //nolint:errcheck
		return fmt.Errorf("oci: closing tar: %w", err)
	}
	return out.Close()
}

// buildOCIEnv builds the OCI image Env from a lockfile's layers.
func buildOCIEnv(lf *spec.LockFile) []string {
	const mergedPath = "/strata/env"

	lastVersionOf := make(map[string]string)
	for _, layer := range lf.Layers {
		if layer.InstallLayout != "flat" {
			lastVersionOf[layer.Name] = layer.Version
		}
	}

	var pathParts, ldParts []string
	for _, layer := range lf.Layers {
		if layer.InstallLayout == "flat" || lastVersionOf[layer.Name] != layer.Version {
			continue
		}
		base := fmt.Sprintf("%s/%s/%s", mergedPath, layer.Name, layer.Version)
		pathParts = append(pathParts, base+"/bin")
		ldParts = append(ldParts, base+"/lib", base+"/lib64")
	}

	var env []string
	if len(pathParts) > 0 {
		env = append(env, "PATH="+strings.Join(pathParts, ":")+":"+"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}
	if len(ldParts) > 0 {
		env = append(env, "LD_LIBRARY_PATH="+strings.Join(ldParts, ":"))
	}
	env = append(env, "STRATA_PROFILE="+lf.ProfileName)
	env = append(env, "STRATA_ENV="+mergedPath)
	for k, v := range lf.Env {
		env = append(env, k+"="+v)
	}
	return env
}

// buildOCILabels builds OCI labels recording Strata provenance.
func buildOCILabels(lf *spec.LockFile, paths []overlay.LayerPath) map[string]string {
	labels := map[string]string{
		"org.opencontainers.image.revision": lf.EnvironmentID(),
	}
	if lf.RekorEntry != "" {
		labels["strata.lockfile.rekor_entry"] = lf.RekorEntry
	}
	for _, lp := range paths {
		// Find the matching layer manifest for rekor entry.
		for _, layer := range lf.Layers {
			if layer.ID == lp.ID {
				if layer.SHA256 != "" {
					labels["strata.layer."+layer.Name+".sha256"] = layer.SHA256
				}
				if layer.RekorEntry != "" {
					labels["strata.layer."+layer.Name+".rekor_entry"] = layer.RekorEntry
				}
				break
			}
		}
	}
	return labels
}
