package module

const (
	Name = "datasciencepipelines"

	ConditionArgoWorkflowAvailable = "ArgoWorkflowAvailable"

	DataSciencePipelinesDoesntOwnArgoCRDReason        = "DataSciencePipelinesDoesntOwnArgoCRD"
	DataSciencePipelinesArgoWorkflowsNotManagedReason = "DataSciencePipelinesArgoWorkflowsNotManaged"
	DataSciencePipelinesArgoWorkflowsCRDMissingReason = "DataSciencePipelinesArgoWorkflowsCRDMissing"

	DataSciencePipelinesDoesntOwnArgoCRDMessage = "" +
		"Failed upgrade: workflows.argoproj.io CRD already exists " +
		"but not deployed by this operator " +
		"(missing/incorrect label component.opendatahub.io/data-science-pipelines-operator=true)"
	DataSciencePipelinesArgoWorkflowsNotManagedMessage = "Argo Workflows controllers are not managed by this operator"
	DataSciencePipelinesArgoWorkflowsCRDMissingMessage = "" +
		"Argo Workflows controllers are not managed by this operator, " +
		"but the CRD is missing"
)
