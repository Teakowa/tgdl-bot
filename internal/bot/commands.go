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
)

type ParsedCommand struct {
	Name   Command
	TaskID string
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
		if len(fields) > 1 {
			out.TaskID = fields[1]
		}
		return out
	case string(CommandRetry):
		out := ParsedCommand{Name: CommandRetry}
		if len(fields) > 1 {
			out.TaskID = fields[1]
		}
		return out
	default:
		return ParsedCommand{Name: CommandUnknown}
	}
}
