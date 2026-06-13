package module

const (
	Name               = "workbenches"
	OperatorConfigName = "opendatahub-workbenches-config"

	OwnedNamespaceLabel            = "opendatahub.io/generated-namespace"
	DefaultNotebooksNamespaceODH   = "opendatahub"
	DefaultNotebooksNamespaceRHOAI = "rhods-notebooks"

	KueueQueueNameLabel        = "kueue.x-k8s.io/queue-name"
	KueueManagedLabelKey       = "kueue.openshift.io/managed"
	KueueLegacyManagedLabelKey = "kueue-managed"

	ConditionImageStreamsAvailable          = "ImageStreamsAvailable"
	ConditionImageStreamsNotAvailableReason = "ImageStreamsNotReady"
)
