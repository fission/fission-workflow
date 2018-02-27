package workflow

import (
	"github.com/fission/fission-workflows/pkg/api/workflow"
	"github.com/fission/fission-workflows/pkg/types"
)

//
// Workflow-specific actions
//

type ActionParseWorkflow struct {
	WfApi *workflow.Api
	Wf    *types.Workflow
}

func (ac *ActionParseWorkflow) Id() string {
	return ac.Wf.Metadata.Id
}

func (ac *ActionParseWorkflow) Apply() error {
	_, err := ac.WfApi.Parse(ac.Wf)
	return err
}
