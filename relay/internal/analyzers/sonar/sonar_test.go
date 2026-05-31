package sonar

import "testing"

func TestParseJavaMajor(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"temurin21", `openjdk version "21.0.5" 2024-10-15 LTS`, 21},
		{"oracle17", `java version "17.0.12" 2024-07-16 LTS`, 17},
		{"legacy8", `openjdk version "1.8.0_392"`, 8},
		{"early-access", `openjdk version "23-ea" 2024-09-17`, 23},
		{"junk", `not a java line`, 0},
		{"empty", ``, 0},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if got := parseJavaMajor(c.in); got != c.want {
				t.Errorf("parseJavaMajor(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestMinJavaMajorMatchesSonarLint(t *testing.T) {
	// SonarLint Core 10.x requires Java 17. If the constant ever drifts
	// below the runtime requirement the analyzer will silently fail at
	// engine init — this test catches that at compile/test time.
	if MinJavaMajor < 17 {
		t.Fatalf("MinJavaMajor=%d is below SonarLint Core 10.x's Java 17 requirement", MinJavaMajor)
	}
}
