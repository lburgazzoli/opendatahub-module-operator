package module

const (
	Name               = "trainer"
	OperatorConfigName = "odh-trainer-config"

	ConditionDependenciesAvailable = "DependenciesAvailable"
	PreConditionFailedReason       = "PreConditionFailed"

	JobSetOperatorNotInstalledMessage = "JobSet operator not installed, please install it first"
	JobSetCRDMissingMessage           = "" +
		"JobSet CRD does not exist, please inspect JobSetOperator CR status conditions " +
		"or JobSet controller Pod logs for more details"
	JobSetOperatorCRNotFoundMessage = "JobSetOperator CR with name 'cluster' not found, please create it first"
)
