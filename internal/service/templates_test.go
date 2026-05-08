package service

import "testing"

func TestParseGitHubTemplateRefWithSubpath(t *testing.T) {
	repo, branch, subpath, err := parseGitHubTemplateRef("https://github.com/fyltr/django-angee/examples/angee-notes/.templates/stack/staging")
	if err != nil {
		t.Fatalf("parseGitHubTemplateRef() error = %v", err)
	}
	if repo != "https://github.com/fyltr/django-angee.git" {
		t.Fatalf("repo = %q", repo)
	}
	if branch != "" {
		t.Fatalf("branch = %q, want empty", branch)
	}
	wantSubpath := "examples/angee-notes/.templates/stack/staging"
	if subpath != wantSubpath {
		t.Fatalf("subpath = %q, want %q", subpath, wantSubpath)
	}
}

func TestParseGitHubTemplateRefWithTreeBranch(t *testing.T) {
	_, branch, subpath, err := parseGitHubTemplateRef("https://github.com/fyltr/django-angee/tree/main/examples/angee-notes/.templates/stacks/dev")
	if err != nil {
		t.Fatalf("parseGitHubTemplateRef() error = %v", err)
	}
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
	wantSubpath := "examples/angee-notes/.templates/stacks/dev"
	if subpath != wantSubpath {
		t.Fatalf("subpath = %q, want %q", subpath, wantSubpath)
	}
}
