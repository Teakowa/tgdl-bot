package util

import "testing"

func TestExtractFirstTelegramURL(t *testing.T) {
	text := "请转发这个链接 https://t.me/c/123456/789 以及其他文本"
	got, ok := ExtractFirstTelegramURL(text)
	if !ok {
		t.Fatal("expected URL to be extracted")
	}
	if got != "https://t.me/c/123456/789" {
		t.Fatalf("unexpected URL: %q", got)
	}
}

func TestExtractFirstTelegramURL_RejectsInvalid(t *testing.T) {
	_, ok := ExtractFirstTelegramURL("https://example.com/a/b")
	if ok {
		t.Fatal("expected invalid URL to be rejected")
	}
}

func TestExtractFirstTelegramURL_TelegramMe(t *testing.T) {
	got, ok := ExtractFirstTelegramURL("x https://telegram.me/channel_name/42 y")
	if !ok {
		t.Fatal("expected telegram.me URL to be extracted")
	}
	if got != "https://telegram.me/channel_name/42" {
		t.Fatalf("unexpected URL: %q", got)
	}
}
