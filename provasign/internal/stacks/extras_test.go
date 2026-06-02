package stacks

import "testing"

func TestIsKnown(t *testing.T) {
	if !IsKnown("go-microservice") {
		t.Error("expected go-microservice known")
	}
	if IsKnown("not-real") {
		t.Error("expected unknown to be false")
	}
}

func TestDescriptionOf_Fallback(t *testing.T) {
	if got := descriptionOf("custom-stack-name"); got != "custom stack name" {
		t.Errorf("got %q", got)
	}
	// Known names hit specific cases.
	for _, n := range []string{"go-microservice", "node-api", "python-service", "java-spring"} {
		if descriptionOf(n) == "" {
			t.Errorf("expected description for %q", n)
		}
	}
}
