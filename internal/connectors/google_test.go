package connectors

import (
	"slices"
	"testing"
)

func TestGoogleModulesAndScopes(t *testing.T) {
	modules := GoogleModules("tasks,gmail,drive,tasks")
	want := []string{GoogleModuleGmail, GoogleModuleDrive, GoogleModuleTasks}
	if !slices.Equal(modules, want) {
		t.Fatalf("unexpected modules: %#v", modules)
	}
	scopes := GoogleScopes(modules)
	for _, scope := range []string{
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/drive.activity.readonly",
		"https://www.googleapis.com/auth/tasks.readonly",
	} {
		if !slices.Contains(scopes, scope) {
			t.Fatalf("missing scope %q in %#v", scope, scopes)
		}
	}
	if slices.Contains(scopes, "https://www.googleapis.com/auth/chat.messages.readonly") {
		t.Fatalf("unselected Chat scope leaked into %#v", scopes)
	}
}

func TestValidGoogleModules(t *testing.T) {
	if !ValidGoogleModules("gmail,calendar,drive,tasks") {
		t.Fatal("supported modules should be valid")
	}
	if ValidGoogleModules("") || ValidGoogleModules("youtube") ||
		ValidGoogleModules("gmail,unknown") {
		t.Fatal("empty or unsupported module selections must be rejected")
	}
}
