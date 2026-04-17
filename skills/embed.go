package skillsassets

import "embed"

// FS exposes the canonical skill source files that are embedded into the awiki-cli binary.
//
//go:embed SKILL.md README.md manifests/skills.yaml references/*.md
var FS embed.FS
