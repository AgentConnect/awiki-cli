package skillbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	skillsassets "github.com/agentconnect/awiki-cli/skills"
	"gopkg.in/yaml.v3"
)

type skillsManifest struct {
	Version string `yaml:"version"`
	Entry   struct {
		Name        string `yaml:"name"`
		Path        string `yaml:"path"`
		Type        string `yaml:"type"`
		Description string `yaml:"description"`
	} `yaml:"entry"`
	References []struct {
		Name              string   `yaml:"name"`
		Path              string   `yaml:"path"`
		Type              string   `yaml:"type"`
		ImplementedStatus string   `yaml:"implemented_status"`
		Description       string   `yaml:"description"`
		Keywords          []string `yaml:"keywords"`
	} `yaml:"references"`
}

var (
	bundleOnce sync.Once
	bundleInst *Bundle
	bundleErr  error
)

func Load() (*Bundle, error) {
	bundleOnce.Do(func() {
		bundleInst, bundleErr = buildBundle()
	})
	return bundleInst, bundleErr
}

func buildBundle() (*Bundle, error) {
	manifestRaw, err := fs.ReadFile(skillsassets.FS, "manifests/skills.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded skills manifest: %w", err)
	}
	var parsed skillsManifest
	if err := yaml.Unmarshal(manifestRaw, &parsed); err != nil {
		return nil, fmt.Errorf("parse embedded skills manifest: %w", err)
	}
	rootPath := trimSkillPrefix(strings.TrimSpace(parsed.Entry.Path))
	if rootPath == "" {
		rootPath = "SKILL.md"
	}
	rootRaw, err := fs.ReadFile(skillsassets.FS, rootPath)
	if err != nil {
		return nil, fmt.Errorf("read embedded root skill: %w", err)
	}
	children := make([]ChildDocMeta, 0, len(parsed.References))
	checksumInputs := []string{rootPath + ":" + checksum(rootRaw)}
	for _, ref := range parsed.References {
		path := trimSkillPrefix(strings.TrimSpace(ref.Path))
		if !strings.HasPrefix(path, "references/") {
			continue
		}
		raw, err := fs.ReadFile(skillsassets.FS, path)
		if err != nil {
			return nil, fmt.Errorf("read embedded reference %s: %w", path, err)
		}
		child := ChildDocMeta{
			Path:    path,
			Kind:    "reference",
			Title:   strings.TrimSpace(ref.Name),
			Summary: strings.TrimSpace(ref.Description),
			SHA256:  checksum(raw),
			Tags:    normalizeTags(ref.Type, ref.Keywords),
		}
		children = append(children, child)
		checksumInputs = append(checksumInputs, child.Path+":"+child.SHA256)
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	sort.Strings(checksumInputs)
	manifest := Manifest{
		SchemaVersion: 1,
		CLIName:       "awiki-cli",
		CLIVersion:    strings.TrimSpace(buildinfo.Version),
		BundleVersion: bundleVersion(),
		GeneratedAt:   strings.TrimSpace(buildinfo.BuildDate),
		BundleSHA256:  checksum([]byte(strings.Join(checksumInputs, "\n"))),
		RootSkill: RootSkillMeta{
			Name:        "awiki-cli",
			Path:        rootPath,
			SHA256:      checksum(rootRaw),
			Description: strings.TrimSpace(parsed.Entry.Description),
		},
		Children: children,
		Metadata: ManifestMeta{
			DefaultSchemaLookup: "awiki-cli schema <resource>.<method>",
			SupportsExport:      true,
		},
	}
	return &Bundle{Manifest: manifest, FS: skillsassets.FS}, nil
}

func trimSkillPrefix(path string) string {
	return strings.TrimPrefix(strings.TrimSpace(path), "skills/")
}

func normalizeTags(kind string, keywords []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(keywords)+1)
	if trimmed := strings.TrimSpace(kind); trimmed != "" {
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	for _, keyword := range keywords {
		trimmed := strings.TrimSpace(keyword)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func checksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func bundleVersion() string {
	if v := strings.TrimSpace(buildinfo.Version); v != "" {
		return v
	}
	return "dev"
}
