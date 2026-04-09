package main

import "testing"

func TestNormalizeWorkspaceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "subdomain only", input: "myteam", want: "myteam"},
		{name: "domain", input: "myteam.slack.com", want: "myteam"},
		{name: "https url", input: "https://myteam.slack.com", want: "myteam"},
		{name: "url with path", input: "https://myteam.slack.com/messages", want: "myteam"},
		{name: "empty", input: "   ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeWorkspaceName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeWorkspaceURL(t *testing.T) {
	got, err := normalizeWorkspaceURL("myteam.slack.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://myteam.slack.com"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
