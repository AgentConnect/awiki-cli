package skillbundle

import (
	"bytes"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/agentconnect/awiki-cli/internal/buildinfo"
	"gopkg.in/yaml.v3"
)

const bootstrapURL = "https://awiki.ai/skills/awiki-cli/SKILL.md"

func RenderRootSkill(bundle *Bundle) (string, string, error) {
	if bundle == nil {
		return "", "", fmt.Errorf("skill bundle is required")
	}
	raw, err := fs.ReadFile(bundle.FS, bundle.Manifest.RootSkill.Path)
	if err != nil {
		return "", "", fmt.Errorf("read root skill template: %w", err)
	}
	rendered, err := renderRootContent(string(raw), bundle)
	if err != nil {
		return "", "", err
	}
	return rendered, checksum([]byte(rendered)), nil
}

func renderRootContent(raw string, bundle *Bundle) (string, error) {
	frontmatter, body := splitFrontmatter(raw)
	if len(frontmatter) != 0 {
		var meta map[string]any
		if err := yaml.Unmarshal(frontmatter, &meta); err != nil {
			return "", fmt.Errorf("parse root skill frontmatter: %w", err)
		}
		meta["name"] = "awiki-cli"
		meta["version"] = bundle.Manifest.BundleVersion
		metadataMap, _ := meta["metadata"].(map[string]any)
		if metadataMap == nil {
			metadataMap = map[string]any{}
		}
		metadataMap["public_skill_name"] = "awiki-cli"
		metadataMap["bootstrap_url"] = bootstrapURL
		metadataMap["cli_version"] = strings.TrimSpace(buildinfo.Version)
		metadataMap["bundle_version"] = bundle.Manifest.BundleVersion
		meta["metadata"] = metadataMap
		encoded, err := yaml.Marshal(meta)
		if err != nil {
			return "", fmt.Errorf("marshal root skill frontmatter: %w", err)
		}
		raw = strings.TrimRight("---\n"+string(encoded)+"---\n\n"+body, "\n")
	} else {
		raw = strings.TrimRight(raw, "\n")
	}
	section := buildGeneratedRootSection(bundle)
	return strings.TrimRight(raw, "\n") + "\n\n" + section + "\n", nil
}

func buildGeneratedRootSection(bundle *Bundle) string {
	children := append([]ChildDocMeta(nil), bundle.Manifest.Children...)
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	lines := []string{
		"---",
		"",
		"## awiki-cli Bootstrap and Dynamic Loading",
		"",
		fmt.Sprintf("- Public skill name: `%s`", bundle.Manifest.RootSkill.Name),
		fmt.Sprintf("- CLI version: `%s`", strings.TrimSpace(buildinfo.Version)),
		fmt.Sprintf("- Bundle version: `%s`", bundle.Manifest.BundleVersion),
		fmt.Sprintf("- Bootstrap URL: `%s`", bootstrapURL),
		"",
		"### Bootstrap",
		"",
		"1. Check whether `awiki-cli` is available on PATH.",
		"2. If it is missing, install it first:",
		"",
		"```bash",
		"npm install -g @awiki/cli",
		"```",
		"",
		"3. Verify the installation:",
		"",
		"```bash",
		"awiki-cli version",
		"```",
		"",
		"4. Then continue to load deeper docs through the CLI itself:",
		"",
		"```bash",
		"awiki-cli skill index --json",
		"awiki-cli skill get <path>",
		"awiki-cli schema <resource>.<method>",
		"```",
		"",
		"### Embedded references",
		"",
	}
	for _, child := range children {
		lines = append(lines, fmt.Sprintf("- `%s` — %s", child.Path, child.Summary))
	}
	return strings.Join(lines, "\n")
}

func splitFrontmatter(raw string) ([]byte, string) {
	if !strings.HasPrefix(raw, "---\n") {
		return nil, raw
	}
	rest := strings.TrimPrefix(raw, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return nil, raw
	}
	frontmatter := rest[:idx]
	body := rest[idx+len("\n---\n"):]
	return []byte(frontmatter), body
}

func rootPreview(bundle *Bundle) (IndexResult, error) {
	rendered, renderedSHA, err := RenderRootSkill(bundle)
	if err != nil {
		return IndexResult{}, err
	}
	root := bundle.Manifest.RootSkill
	root.SHA256 = renderedSHA
	root.Path = "SKILL.md"
	root.Description = strings.TrimSpace(root.Description)
	_ = rendered
	return IndexResult{
		Name:          root.Name,
		CLIVersion:    bundle.Manifest.CLIVersion,
		BundleVersion: bundle.Manifest.BundleVersion,
		RootSkill:     root,
		Children:      append([]ChildDocMeta(nil), bundle.Manifest.Children...),
	}, nil
}

func renderedRootBytes(bundle *Bundle) ([]byte, string, error) {
	rendered, sha, err := RenderRootSkill(bundle)
	if err != nil {
		return nil, "", err
	}
	return bytes.Clone([]byte(rendered)), sha, nil
}
