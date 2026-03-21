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
		{in: "/status abc-123", want: ParsedCommand{Name: CommandStatus, TaskID: "abc-123"}},
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
}
