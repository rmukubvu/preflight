package deploy

import "testing"

func TestFindPulumiWithLookPath(t *testing.T) {
	bin, err := findPulumiWithLookPath(func(name string) (string, error) {
		if name != "pulumi" {
			t.Fatalf("unexpected lookup: %s", name)
		}
		return "/usr/local/bin/pulumi", nil
	})
	if err != nil {
		t.Fatalf("findPulumiWithLookPath: %v", err)
	}
	if bin != "/usr/local/bin/pulumi" {
		t.Fatalf("want pulumi path, got %q", bin)
	}
}
