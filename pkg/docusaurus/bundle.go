package docusaurus

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GenerateSharingManifest constructs and writes a sharing-manifest.json file with SHA-256 checksums.
func GenerateSharingManifest(siteDir string, cfg *SiteConfig) (*SharingManifest, error) {
	if cfg == nil {
		return nil, ErrInvalidConfig
	}
	docsDir := filepath.Join(siteDir, "docs")
	if _, statErr := os.Stat(docsDir); os.IsNotExist(statErr) {
		fallback := filepath.Join(filepath.Dir(siteDir), "docs")
		if _, statErr := os.Stat(fallback); statErr == nil {
			docsDir = fallback
		}
	}
	checksums, categories, err := computeDocChecksumsAndCategories(docsDir)
	if err != nil {
		return nil, fmt.Errorf("failed computing checksums: %w", err)
	}

	commit, branch, commitDate := probeGitMetadata()

	manifest := &SharingManifest{
		ManifestVersion: "1.0.0",
		ProjectName:     cfg.ProjectName,
		Version:         "1.0.0",
		Description:     cfg.Tagline,
		Repository:      cfg.GitRepo,
		CommitSHA:       commit,
		CommitDate:      commitDate,
		Branch:          branch,
		ExportedAt:      time.Now().UTC(),
		Generator:       "Dendrite Docusaurus Engine v1.0.0",
		SharingTier:     cfg.SharingTier,
		TotalDocuments:  len(checksums),
		Categories:      categories,
		Federation:      cfg.Federation,
		FileChecksums:   checksums,
		ContainerImage:  fmt.Sprintf("%s-docs-portal:latest", cfg.ProjectName),
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}

	manifestPath := filepath.Join(siteDir, "sharing-manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed writing manifest: %w", err)
	}

	return manifest, nil
}

func computeDocChecksumsAndCategories(docsDir string) (map[string]string, []string, error) {
	checksums := make(map[string]string)
	catSet := make(map[string]struct{})

	err := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		rel, err := filepath.Rel(docsDir, path)
		if err != nil {
			return err
		}

		dirPart := filepath.Dir(rel)
		if dirPart != "." && dirPart != "" {
			catSet[dirPart] = struct{}{}
		}

		hash, err := calculateSHA256(path)
		if err != nil {
			return err
		}
		checksums[rel] = hash
		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	categories := make([]string, 0, len(catSet))
	for cat := range catSet {
		categories = append(categories, cat)
	}
	return checksums, categories, nil
}

func calculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func probeGitMetadata() (string, string, string) {
	commit := runGitCmd("rev-parse", "HEAD")
	if commit == "" {
		commit = "unknown-commit"
	}
	branch := runGitCmd("rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" {
		branch = "main"
	}
	date := runGitCmd("log", "-1", "--format=%cd", "--date=iso")
	if date == "" {
		date = time.Now().UTC().Format(time.RFC3339)
	}
	return commit, branch, date
}

func runGitCmd(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ExportSharingArchive writes a compressed tar.gz bundle of the specified directory.
func ExportSharingArchive(sourceDir, destBundlePath string) error {
	if sourceDir == "" || destBundlePath == "" {
		return ErrBundleExportFailed
	}

	cleanDest, _ := filepath.Abs(destBundlePath)

	if err := os.MkdirAll(filepath.Dir(destBundlePath), 0755); err != nil {
		return fmt.Errorf("%w: cannot create bundle dir: %v", ErrBundleExportFailed, err)
	}

	tarFile, err := os.Create(destBundlePath)
	if err != nil {
		return fmt.Errorf("%w: cannot create tar file: %v", ErrBundleExportFailed, err)
	}
	defer tarFile.Close()

	gzWriter := gzip.NewWriter(tarFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil || rel == "." {
			return err
		}

		// Prevent recursive archiving of destination bundle, large build outputs, or node_modules
		cleanPath, _ := filepath.Abs(path)
		if cleanPath == cleanDest || strings.HasSuffix(rel, ".tar.gz") || strings.HasSuffix(rel, ".zip") {
			return nil
		}
		if strings.HasPrefix(rel, "node_modules") || strings.HasPrefix(rel, ".git") || strings.HasPrefix(rel, "build") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	})
}
