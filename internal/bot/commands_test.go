package bot

import "testing"

func TestParseCommand(t *testing.T) {
	cases := []struct {
		in   string
		want ParsedCommand
	}{
		{in: "/start", want: ParsedCommand{Name: CommandStart}},
		{in: "/help", want: ParsedCommand{Name: CommandHelp}},
		{in: "/last", want: ParsedCommand{Name: CommandLast}},
		{in: "/queue", want: ParsedCommand{Name: CommandQueue}},
		{in: "/status abc-123", want: ParsedCommand{Name: CommandStatus, TaskID: "abc-123"}},
		{in: "/forward https://t.me/c/1/2 @channel_name", want: ParsedCommand{Name: CommandForward, SourceURL: "https://t.me/c/1/2", TargetPeer: "@channel_name"}},
		{in: "/forward https://t.me/c/1/2 channel_name --drop-caption", want: ParsedCommand{Name: CommandForward, SourceURL: "https://t.me/c/1/2", TargetPeer: "channel_name", DropCaption: true}},
		{in: "/delete abc-123", want: ParsedCommand{Name: CommandDelete, TaskID: "abc-123"}},
		{in: "/delete abc-123 -f", want: ParsedCommand{Name: CommandDelete, TaskID: "abc-123", Force: true}},
		{in: "/delete --force abc-123", want: ParsedCommand{Name: CommandDelete, TaskID: "abc-123", Force: true}},
		{in: "/delete -f abc-123", want: ParsedCommand{Name: CommandDelete, TaskID: "abc-123", Force: true}},
		{in: "/retry abc-123", want: ParsedCommand{Name: CommandRetry, TaskID: "abc-123"}},
		{in: "https://t.me/c/1/2", want: ParsedCommand{Name: CommandUnknown}},
	}

	for _, c := range cases {
		got := ParseCommand(c.in)
		if got != c.want {
			t.Fatalf("parse %q: got %+v want %+v", c.in, got, c.want)
		}
	}
}

func TestBuildCommandReply(t *testing.T) {
	if got := BuildCommandReply(ParsedCommand{Name: CommandStatus}); got == "" {
		t.Fatal("expected usage reply for /status without task id")
	}
	if got := BuildCommandReply(ParsedCommand{Name: CommandStart}); got == "" {
		t.Fatal("expected /start reply")
	}
	if got := BuildCommandReply(ParsedCommand{Name: CommandDelete}); got != "用法: /delete [task_id] [-f|--force]" {
		t.Fatalf("unexpected delete usage: %s", got)
	}
	if got := BuildCommandReply(ParsedCommand{Name: CommandForward}); got != "用法: /forward <source_url> <target> [--drop-caption]" {
		t.Fatalf("unexpected forward usage: %s", got)
	}
}
