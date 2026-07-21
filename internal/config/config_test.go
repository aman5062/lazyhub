package config

import "testing"

// TestAuthRoundtrip verifies that a saved credential can be loaded back and
// then cleared. In an environment without an OS keychain (CI, headless), this
// exercises the file-fallback path; where a keychain exists it exercises that.
func TestAuthRoundtrip(t *testing.T) {
	// Point config storage at a throwaway dir for this test.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, err := LoadAuth(); err != ErrNoAuth {
		t.Fatalf("expected ErrNoAuth before login, got %v", err)
	}

	in := &Auth{Token: "ghp_example_token", Method: "pat", Login: "octocat", Scopes: "project repo"}
	if err := SaveAuth(in); err != nil {
		t.Fatalf("SaveAuth: %v", err)
	}
	// SaveAuth must not mutate the caller's token (it copies before blanking).
	if in.Token == "" {
		t.Fatal("SaveAuth cleared the caller's token")
	}

	got, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	if got.Token != in.Token || got.Login != in.Login || got.Method != in.Method {
		t.Fatalf("roundtrip mismatch: got %+v want token/login/method from %+v", got, in)
	}

	if err := ClearAuth(); err != nil {
		t.Fatalf("ClearAuth: %v", err)
	}
	if _, err := LoadAuth(); err != ErrNoAuth {
		t.Fatalf("expected ErrNoAuth after logout, got %v", err)
	}
}
