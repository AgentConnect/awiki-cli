package skillbundle

import (
	"fmt"
	"io/fs"
)

func Validate() error {
	bundle, err := Load()
	if err != nil {
		return err
	}
	if bundle.Manifest.RootSkill.Name != "awiki-cli" {
		return fmt.Errorf("root skill name = %q, want awiki-cli", bundle.Manifest.RootSkill.Name)
	}
	if _, err := fs.ReadFile(bundle.FS, bundle.Manifest.RootSkill.Path); err != nil {
		return fmt.Errorf("root skill path %s is missing: %w", bundle.Manifest.RootSkill.Path, err)
	}
	seen := map[string]struct{}{}
	for _, child := range bundle.Manifest.Children {
		if _, ok := seen[child.Path]; ok {
			return fmt.Errorf("duplicate skill child path %s", child.Path)
		}
		seen[child.Path] = struct{}{}
		if _, err := fs.ReadFile(bundle.FS, child.Path); err != nil {
			return fmt.Errorf("skill child path %s is missing: %w", child.Path, err)
		}
	}
	return nil
}
