package bot

import "testing"

func TestExtractTaskURL(t *testing.T) {
	got, ok := ExtractTaskURL("hello https://t.me/channel_1/12 world")
	if !ok {
		t.Fatal("expected valid task url")
	}
	if got != "https://t.me/channel_1/12" {
		t.Fatalf("unexpected url: %q", got)
	}
}

func TestIsAllowedUser(t *testing.T) {
	if !IsAllowedUser(nil, 100) {
		t.Fatal("empty allowlist should allow all")
	}
	if IsAllowedUser([]int64{1, 2, 3}, 9) {
		t.Fatal("unexpected allow for non-whitelisted user")
	}
	if !IsAllowedUser([]int64{1, 2, 3}, 2) {
		t.Fatal("expected user in whitelist to be allowed")
	}
}
