package flow

type MessageKind string

const (
	MessageAssistant  MessageKind = "assistant"
	MessagePartial    MessageKind = "partial"
	MessageToolCall   MessageKind = "tool_call"
	MessageToolResult MessageKind = "tool_result"
	MessageError      MessageKind = "error"
	MessageDone       MessageKind = "done"
)

type Message interface {
	Kind() MessageKind
}

type AssistantMessage struct {
	Answer string
}

func (*AssistantMessage) Kind() MessageKind {
	return MessageAssistant
}

type PartialMessage struct {
	Text string
}

func (*PartialMessage) Kind() MessageKind {
	return MessagePartial
}

type ToolCallMessage struct {
	Tool       string
	Input      map[string]any
	Iteration  int
	ToolCallID string
}

func (*ToolCallMessage) Kind() MessageKind {
	return MessageToolCall
}

type ToolResultMessage struct {
	Tool        string
	Input       map[string]any
	Observation string
	Error       string
	Iteration   int
	ToolCallID  string
}

func (*ToolResultMessage) Kind() MessageKind {
	return MessageToolResult
}

type ErrorMessage struct {
	Message string
	Partial string
	Err     error
}

func (*ErrorMessage) Kind() MessageKind {
	return MessageError
}

type DoneMessage struct{}

func (*DoneMessage) Kind() MessageKind {
	return MessageDone
}
