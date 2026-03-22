package bot

import "strings"

type Command string

const (
	CommandUnknown Command = ""
	CommandStart   Command = "/start"
	CommandHelp    Command = "/help"
	CommandStatus  Command = "/status"
	CommandLast    Command = "/last"
	CommandQueue   Command = "/queue"
	CommandDelete  Command = "/delete"
	CommandRetry   Command = "/retry"
	CommandForward Command = "/forward"
)

type ParsedCommand struct {
	Name        Command
	TaskID      string
	Force       bool
	SourceURL   string
	TargetPeer  string
	DropCaption bool
}

func ParseCommand(text string) ParsedCommand {
	input := strings.TrimSpace(text)
	if input == "" || !strings.HasPrefix(input, "/") {
		return ParsedCommand{Name: CommandUnknown}
	}

	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ParsedCommand{Name: CommandUnknown}
	}

	switch fields[0] {
	case string(CommandStart):
		return ParsedCommand{Name: CommandStart}
	case string(CommandHelp):
		return ParsedCommand{Name: CommandHelp}
	case string(CommandLast):
		return ParsedCommand{Name: CommandLast}
	case string(CommandQueue):
		return ParsedCommand{Name: CommandQueue}
	case string(CommandStatus):
		out := ParsedCommand{Name: CommandStatus}
		if len(fields) > 1 {
			out.TaskID = fields[1]
		}
		return out
	case string(CommandDelete):
		out := ParsedCommand{Name: CommandDelete}
		out.TaskID, out.Force = parseDeleteArgs(fields[1:])
		return out
	case string(CommandRetry):
		out := ParsedCommand{Name: CommandRetry}
		if len(fields) > 1 {
			out.TaskID = fields[1]
		}
		return out
	case string(CommandForward):
		return parseForwardCommand(fields[1:])
	default:
		return ParsedCommand{Name: CommandUnknown}
	}
}

func parseDeleteArgs(args []string) (string, bool) {
	taskID := ""
	force := false
	for _, arg := range args {
		switch arg {
		case "-f", "--force":
			force = true
		default:
			if taskID == "" {
				taskID = arg
			}
		}
	}
	return taskID, force
}

func parseForwardCommand(args []string) ParsedCommand {
	out := ParsedCommand{Name: CommandForward}
	positional := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "--drop-caption":
			out.DropCaption = true
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) > 0 {
		out.SourceURL = positional[0]
	}
	if len(positional) > 1 {
		out.TargetPeer = positional[1]
	}
	return out
}
