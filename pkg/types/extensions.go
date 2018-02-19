package types

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
)

// Types other than specified in protobuf
const (
	SUBJECT_INVOCATION = "invocation"
	SUBJECT_WORKFLOW   = "workflows"
	INPUT_MAIN         = "default"
	INPUT_HEADERS      = "headers"
	INPUT_QUERY        = "query"
	INPUT_METHOD       = "method"

	typedValueShortMaxLen = 32
	WorkflowApiVersion    = "v1"
)

// InvocationEvent
var invocationFinalStates = []WorkflowInvocationStatus_Status{
	WorkflowInvocationStatus_ABORTED,
	WorkflowInvocationStatus_SUCCEEDED,
	WorkflowInvocationStatus_FAILED,
}

var taskFinalStates = []TaskInvocationStatus_Status{
	TaskInvocationStatus_FAILED,
	TaskInvocationStatus_ABORTED,
	TaskInvocationStatus_SKIPPED,
	TaskInvocationStatus_SUCCEEDED,
}

func (wi WorkflowInvocationStatus) Finished() bool {
	for _, event := range invocationFinalStates {
		if event == wi.Status {
			return true
		}
	}
	return false
}

func (wi WorkflowInvocationStatus) Successful() bool {
	return wi.Status == WorkflowInvocationStatus_SUCCEEDED
}

func (ti TaskInvocationStatus) Finished() bool {
	for _, event := range taskFinalStates {
		if event == ti.Status {
			return true
		}
	}
	return false
}

// Prints a short description of the value
func (tv TypedValue) Short() string {
	var val string
	if len(tv.Value) > typedValueShortMaxLen {
		val = fmt.Sprintf("%s[..%d..]", tv.Value[:typedValueShortMaxLen], len(tv.Value)-typedValueShortMaxLen)
	} else {
		val = fmt.Sprintf("%s", tv.Value)
	}

	return fmt.Sprintf("<Type=\"%s\", Val=\"%v\">", tv.Type, strings.Replace(val, "\n", "", -1))
}

func (m *Error) Error() string {
	return m.Message
}

// Note: this only retrieves the statically, top-level defined tasks
func (m *Workflow) Task(id string) (*Task, bool) {
	var ok bool
	spec, ok := m.Spec.Tasks[id]
	if !ok {
		return nil, false
	}
	var status *TaskStatus
	if m.Status.Tasks != nil {
		status, ok = m.Status.Tasks[id]
	}
	if !ok {
		status = &TaskStatus{
			UpdatedAt: ptypes.TimestampNow(),
		}
	}

	return &Task{
		Metadata: &ObjectMetadata{
			Id:        id,
			CreatedAt: m.Metadata.CreatedAt,
		},
		Spec:   spec,
		Status: status,
	}, true
}

// Note: this only retrieves the statically top-level defined tasks
func (m *Workflow) Tasks() []*Task {
	var tasks []*Task
	for id := range m.Spec.Tasks {
		task, _ := m.Task(id)
		tasks = append(tasks, task)
	}
	return tasks
}

func (m *Task) Id() string {
	return m.Metadata.Id
}

func (m *WorkflowInvocation) Id() string {
	return m.Metadata.Id
}

func (m *TaskInvocation) Id() string {
	return m.Metadata.Id
}

func (m *Workflow) Id() string {
	return m.Metadata.Id
}

func (m *TaskSpec) Overlay(overlay *TaskSpec) *TaskSpec {
	nt := proto.Clone(m).(*TaskSpec)
	nt.Await = overlay.Await
	nt.Requires = overlay.Requires
	return nt
}

func (m *WorkflowInvocation) TaskInvocation(id string) (*TaskInvocation, bool) {
	ti, ok := m.Status.Tasks[id]
	return ti, ok
}

func (m *WorkflowInvocation) TaskInvocations() []*TaskInvocation {
	var tasks []*TaskInvocation
	for id := range m.Status.Tasks {
		task, _ := m.TaskInvocation(id)
		tasks = append(tasks, task)
	}
	return tasks
}

func (m *TaskSpec) AddDependency(taskId string, opts ...*TaskDependencyParameters) {
	if m.Requires == nil {
		m.Requires = map[string]*TaskDependencyParameters{}
	}
	var params *TaskDependencyParameters
	if len(opts) > 0 {
		params = opts[0]
	}

	m.Requires[taskId] = params
}

func (m *WorkflowSpec) TaskIds() []string {
	var ids []string
	for k := range m.Tasks {
		ids = append(ids, k)
	}
	return ids
}

type WorkflowInstance struct {
	Workflow *Workflow

	// Invocation is nil if not yet invoked
	Invocation *WorkflowInvocation
}

type TaskInstance struct {
	Task *Task
	// Invocation is nil if not yet invoked
	Invocation *TaskInvocation
}

func (m *WorkflowStatus) Ready() bool {
	return m.Status == WorkflowStatus_READY
}

func (m *WorkflowStatus) AddTaskStatus(id string, t *TaskStatus) {
	if m.Tasks == nil {
		m.Tasks = map[string]*TaskStatus{}
	}
	m.Tasks[id] = t
}

func (m *TaskSpec) Parent() (string, bool) {
	var parent string
	var present bool
	for id, params := range m.Requires {
		if params.Type == TaskDependencyParameters_DYNAMIC_OUTPUT {
			present = true
			parent = id
			break
		}
	}
	return parent, present
}

func (m *TaskInvocationSpec) ToWorkflowSpec() *WorkflowInvocationSpec {
	return &WorkflowInvocationSpec{
		WorkflowId: m.Type.Resolved,
		Inputs:     m.Inputs,
	}
}

func (m *WorkflowInvocationStatus) ToTaskStatus() *TaskInvocationStatus {
	var statusMapping = map[WorkflowInvocationStatus_Status]TaskInvocationStatus_Status{
		WorkflowInvocationStatus_UNKNOWN:     TaskInvocationStatus_UNKNOWN,
		WorkflowInvocationStatus_SCHEDULED:   TaskInvocationStatus_SCHEDULED,
		WorkflowInvocationStatus_IN_PROGRESS: TaskInvocationStatus_IN_PROGRESS,
		WorkflowInvocationStatus_SUCCEEDED:   TaskInvocationStatus_SUCCEEDED,
		WorkflowInvocationStatus_FAILED:      TaskInvocationStatus_FAILED,
		WorkflowInvocationStatus_ABORTED:     TaskInvocationStatus_ABORTED,
	}

	return &TaskInvocationStatus{
		Status:    statusMapping[m.Status],
		Error:     m.Error,
		UpdatedAt: m.UpdatedAt,
		Output:    m.Output,
	}
}

func (m *TaskSpec) AddInput(key string, val *TypedValue) {
	if len(m.Inputs) == 0 {
		m.Inputs = map[string]*TypedValue{}
	}
	m.Inputs[key] = val
}
