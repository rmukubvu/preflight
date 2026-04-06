package deploy

import (
	"os"
	"path/filepath"
)

// StackType identifies the IaC tool managing the stack.
type StackType string

const (
	StackTypeCDK       StackType = "cdk"
	StackTypeTerraform StackType = "terraform"
	StackTypeUnknown   StackType = ""
)

// DetectStackType infers the stack type from files in dir.
// It looks for cdk.json (CDK) or *.tf files (Terraform).
// Returns StackTypeUnknown when neither is found.
func DetectStackType(dir string) StackType {
	if fileExists(filepath.Join(dir, "cdk.json")) {
		return StackTypeCDK
	}
	if hasTerraformFiles(dir) {
		return StackTypeTerraform
	}
	return StackTypeUnknown
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasTerraformFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".tf" {
			return true
		}
	}
	return false
}
