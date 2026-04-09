package stack

import (
	"os"
	"path/filepath"
)

// Type identifies the IaC tool managing the stack.
type Type string

const (
	TypeCDK       Type = "cdk"
	TypeTerraform Type = "terraform"
	TypeUnknown   Type = ""
)

// Detect infers the stack type from files in dir.
// It looks for cdk.json (CDK) or *.tf files (Terraform).
// Returns TypeUnknown when neither is found.
func Detect(dir string) Type {
	if fileExists(filepath.Join(dir, "cdk.json")) {
		return TypeCDK
	}
	if hasTerraformFiles(dir) {
		return TypeTerraform
	}
	return TypeUnknown
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
