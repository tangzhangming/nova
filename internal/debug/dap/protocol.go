// protocol.go - DAP (Debug Adapter Protocol) 消息定义
//
// 实现 DAP 协议的消息结构。
// 参考：https://microsoft.github.io/debug-adapter-protocol/specification

package dap

// Message DAP 基础消息
type Message struct {
	Seq  int    `json:"seq"`
	Type string `json:"type"` // "request", "response", "event"
}

// Request DAP 请求
type Request struct {
	Message
	Command   string      `json:"command"`
	Arguments interface{} `json:"arguments,omitempty"`
}

// Response DAP 响应
type Response struct {
	Message
	RequestSeq int         `json:"request_seq"`
	Success    bool        `json:"success"`
	Command    string      `json:"command"`
	ErrorMessage string    `json:"message,omitempty"`
	Body       interface{} `json:"body,omitempty"`
}

// Event DAP 事件
type Event struct {
	Message
	Event string      `json:"event"`
	Body  interface{} `json:"body,omitempty"`
}

// ============================================================================
// 请求参数
// ============================================================================

// InitializeRequestArguments 初始化请求参数
type InitializeRequestArguments struct {
	ClientID                     string `json:"clientID,omitempty"`
	ClientName                   string `json:"clientName,omitempty"`
	AdapterID                    string `json:"adapterID"`
	Locale                       string `json:"locale,omitempty"`
	LinesStartAt1                bool   `json:"linesStartAt1,omitempty"`
	ColumnsStartAt1              bool   `json:"columnsStartAt1,omitempty"`
	PathFormat                   string `json:"pathFormat,omitempty"`
	SupportsVariableType         bool   `json:"supportsVariableType,omitempty"`
	SupportsVariablePaging       bool   `json:"supportsVariablePaging,omitempty"`
	SupportsRunInTerminalRequest bool   `json:"supportsRunInTerminalRequest,omitempty"`
}

// LaunchRequestArguments 启动请求参数
type LaunchRequestArguments struct {
	NoDebug    bool   `json:"noDebug,omitempty"`
	Program    string `json:"program"`
	Args       []string `json:"args,omitempty"`
	Cwd        string `json:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	StopOnEntry bool `json:"stopOnEntry,omitempty"`
}

// AttachRequestArguments 附加请求参数
type AttachRequestArguments struct {
	ProcessId int `json:"processId,omitempty"`
}

// SetBreakpointsArguments 设置断点参数
type SetBreakpointsArguments struct {
	Source      Source             `json:"source"`
	Breakpoints []SourceBreakpoint `json:"breakpoints,omitempty"`
	Lines       []int              `json:"lines,omitempty"`
	SourceModified bool            `json:"sourceModified,omitempty"`
}

// SourceBreakpoint 源码断点
type SourceBreakpoint struct {
	Line         int    `json:"line"`
	Column       int    `json:"column,omitempty"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
	LogMessage   string `json:"logMessage,omitempty"`
}

// SetFunctionBreakpointsArguments 设置函数断点参数
type SetFunctionBreakpointsArguments struct {
	Breakpoints []FunctionBreakpoint `json:"breakpoints"`
}

// FunctionBreakpoint 函数断点
type FunctionBreakpoint struct {
	Name         string `json:"name"`
	Condition    string `json:"condition,omitempty"`
	HitCondition string `json:"hitCondition,omitempty"`
}

// ContinueArguments 继续执行参数
type ContinueArguments struct {
	ThreadId int `json:"threadId"`
}

// NextArguments 下一步参数
type NextArguments struct {
	ThreadId int `json:"threadId"`
}

// StepInArguments 步入参数
type StepInArguments struct {
	ThreadId int `json:"threadId"`
	TargetId int `json:"targetId,omitempty"`
}

// StepOutArguments 步出参数
type StepOutArguments struct {
	ThreadId int `json:"threadId"`
}

// PauseArguments 暂停参数
type PauseArguments struct {
	ThreadId int `json:"threadId"`
}

// StackTraceArguments 栈追踪参数
type StackTraceArguments struct {
	ThreadId   int `json:"threadId"`
	StartFrame int `json:"startFrame,omitempty"`
	Levels     int `json:"levels,omitempty"`
}

// ScopesArguments 作用域参数
type ScopesArguments struct {
	FrameId int `json:"frameId"`
}

// VariablesArguments 变量参数
type VariablesArguments struct {
	VariablesReference int    `json:"variablesReference"`
	Filter             string `json:"filter,omitempty"`
	Start              int    `json:"start,omitempty"`
	Count              int    `json:"count,omitempty"`
}

// EvaluateArguments 求值参数
type EvaluateArguments struct {
	Expression string `json:"expression"`
	FrameId    int    `json:"frameId,omitempty"`
	Context    string `json:"context,omitempty"`
}

// ============================================================================
// 响应体
// ============================================================================

// Capabilities 调试器能力
type Capabilities struct {
	SupportsConfigurationDoneRequest   bool `json:"supportsConfigurationDoneRequest,omitempty"`
	SupportsFunctionBreakpoints        bool `json:"supportsFunctionBreakpoints,omitempty"`
	SupportsConditionalBreakpoints     bool `json:"supportsConditionalBreakpoints,omitempty"`
	SupportsHitConditionalBreakpoints  bool `json:"supportsHitConditionalBreakpoints,omitempty"`
	SupportsEvaluateForHovers          bool `json:"supportsEvaluateForHovers,omitempty"`
	ExceptionBreakpointFilters         []ExceptionBreakpointsFilter `json:"exceptionBreakpointFilters,omitempty"`
	SupportsStepBack                   bool `json:"supportsStepBack,omitempty"`
	SupportsSetVariable                bool `json:"supportsSetVariable,omitempty"`
	SupportsRestartFrame               bool `json:"supportsRestartFrame,omitempty"`
	SupportsGotoTargetsRequest         bool `json:"supportsGotoTargetsRequest,omitempty"`
	SupportsStepInTargetsRequest       bool `json:"supportsStepInTargetsRequest,omitempty"`
	SupportsCompletionsRequest         bool `json:"supportsCompletionsRequest,omitempty"`
	SupportsModulesRequest             bool `json:"supportsModulesRequest,omitempty"`
	SupportsRestartRequest             bool `json:"supportsRestartRequest,omitempty"`
	SupportsExceptionOptions           bool `json:"supportsExceptionOptions,omitempty"`
	SupportsValueFormattingOptions     bool `json:"supportsValueFormattingOptions,omitempty"`
	SupportsExceptionInfoRequest       bool `json:"supportsExceptionInfoRequest,omitempty"`
	SupportTerminateDebuggee           bool `json:"supportTerminateDebuggee,omitempty"`
	SupportsDelayedStackTraceLoading   bool `json:"supportsDelayedStackTraceLoading,omitempty"`
	SupportsLoadedSourcesRequest       bool `json:"supportsLoadedSourcesRequest,omitempty"`
	SupportsLogPoints                  bool `json:"supportsLogPoints,omitempty"`
	SupportsTerminateThreadsRequest    bool `json:"supportsTerminateThreadsRequest,omitempty"`
	SupportsSetExpression              bool `json:"supportsSetExpression,omitempty"`
	SupportsTerminateRequest           bool `json:"supportsTerminateRequest,omitempty"`
}

// ExceptionBreakpointsFilter 异常断点过滤器
type ExceptionBreakpointsFilter struct {
	Filter               string `json:"filter"`
	Label                string `json:"label"`
	Description          string `json:"description,omitempty"`
	Default              bool   `json:"default,omitempty"`
	SupportsCondition    bool   `json:"supportsCondition,omitempty"`
	ConditionDescription string `json:"conditionDescription,omitempty"`
}

// SetBreakpointsResponseBody 设置断点响应体
type SetBreakpointsResponseBody struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
}

// Breakpoint 断点信息
type Breakpoint struct {
	Id        int    `json:"id,omitempty"`
	Verified  bool   `json:"verified"`
	Message   string `json:"message,omitempty"`
	Source    *Source `json:"source,omitempty"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	EndColumn int    `json:"endColumn,omitempty"`
}

// ContinueResponseBody 继续执行响应体
type ContinueResponseBody struct {
	AllThreadsContinued bool `json:"allThreadsContinued,omitempty"`
}

// StackTraceResponseBody 栈追踪响应体
type StackTraceResponseBody struct {
	StackFrames []StackFrame `json:"stackFrames"`
	TotalFrames int          `json:"totalFrames,omitempty"`
}

// StackFrame 栈帧
type StackFrame struct {
	Id                          int    `json:"id"`
	Name                        string `json:"name"`
	Source                      *Source `json:"source,omitempty"`
	Line                        int    `json:"line"`
	Column                      int    `json:"column"`
	EndLine                     int    `json:"endLine,omitempty"`
	EndColumn                   int    `json:"endColumn,omitempty"`
	InstructionPointerReference string `json:"instructionPointerReference,omitempty"`
	ModuleId                    interface{} `json:"moduleId,omitempty"`
	PresentationHint            string `json:"presentationHint,omitempty"`
}

// ScopesResponseBody 作用域响应体
type ScopesResponseBody struct {
	Scopes []Scope `json:"scopes"`
}

// Scope 作用域
type Scope struct {
	Name               string `json:"name"`
	PresentationHint   string `json:"presentationHint,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
	Expensive          bool   `json:"expensive,omitempty"`
	Source             *Source `json:"source,omitempty"`
	Line               int    `json:"line,omitempty"`
	Column             int    `json:"column,omitempty"`
	EndLine            int    `json:"endLine,omitempty"`
	EndColumn          int    `json:"endColumn,omitempty"`
}

// VariablesResponseBody 变量响应体
type VariablesResponseBody struct {
	Variables []Variable `json:"variables"`
}

// Variable 变量
type Variable struct {
	Name               string `json:"name"`
	Value              string `json:"value"`
	Type               string `json:"type,omitempty"`
	PresentationHint   *VariablePresentationHint `json:"presentationHint,omitempty"`
	EvaluateName       string `json:"evaluateName,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
}

// VariablePresentationHint 变量展示提示
type VariablePresentationHint struct {
	Kind       string   `json:"kind,omitempty"`
	Attributes []string `json:"attributes,omitempty"`
	Visibility string   `json:"visibility,omitempty"`
}

// EvaluateResponseBody 求值响应体
type EvaluateResponseBody struct {
	Result             string `json:"result"`
	Type               string `json:"type,omitempty"`
	PresentationHint   *VariablePresentationHint `json:"presentationHint,omitempty"`
	VariablesReference int    `json:"variablesReference"`
	NamedVariables     int    `json:"namedVariables,omitempty"`
	IndexedVariables   int    `json:"indexedVariables,omitempty"`
}

// ThreadsResponseBody 线程响应体
type ThreadsResponseBody struct {
	Threads []Thread `json:"threads"`
}

// Thread 线程
type Thread struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

// ============================================================================
// 事件体
// ============================================================================

// InitializedEventBody 初始化完成事件体
type InitializedEventBody struct{}

// StoppedEventBody 停止事件体
type StoppedEventBody struct {
	Reason            string `json:"reason"` // "step", "breakpoint", "exception", "pause", "entry"
	Description       string `json:"description,omitempty"`
	ThreadId          int    `json:"threadId,omitempty"`
	PreserveFocusHint bool   `json:"preserveFocusHint,omitempty"`
	Text              string `json:"text,omitempty"`
	AllThreadsStopped bool   `json:"allThreadsStopped,omitempty"`
}

// ContinuedEventBody 继续事件体
type ContinuedEventBody struct {
	ThreadId            int  `json:"threadId"`
	AllThreadsContinued bool `json:"allThreadsContinued,omitempty"`
}

// ExitedEventBody 退出事件体
type ExitedEventBody struct {
	ExitCode int `json:"exitCode"`
}

// TerminatedEventBody 终止事件体
type TerminatedEventBody struct {
	Restart interface{} `json:"restart,omitempty"`
}

// OutputEventBody 输出事件体
type OutputEventBody struct {
	Category string  `json:"category,omitempty"` // "console", "stdout", "stderr", "telemetry"
	Output   string  `json:"output"`
	Group    string  `json:"group,omitempty"`
	VariablesReference int `json:"variablesReference,omitempty"`
	Source   *Source `json:"source,omitempty"`
	Line     int     `json:"line,omitempty"`
	Column   int     `json:"column,omitempty"`
}

// BreakpointEventBody 断点事件体
type BreakpointEventBody struct {
	Reason     string     `json:"reason"` // "changed", "new", "removed"
	Breakpoint Breakpoint `json:"breakpoint"`
}

// ============================================================================
// 通用类型
// ============================================================================

// Source 源码
type Source struct {
	Name             string `json:"name,omitempty"`
	Path             string `json:"path,omitempty"`
	SourceReference  int    `json:"sourceReference,omitempty"`
	PresentationHint string `json:"presentationHint,omitempty"`
	Origin           string `json:"origin,omitempty"`
	Sources          []Source `json:"sources,omitempty"`
	AdapterData      interface{} `json:"adapterData,omitempty"`
	Checksums        []Checksum `json:"checksums,omitempty"`
}

// Checksum 校验和
type Checksum struct {
	Algorithm string `json:"algorithm"`
	Checksum  string `json:"checksum"`
}
