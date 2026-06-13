package module

const (
	Name               = "ray"
	OperatorConfigName = "opendatahub-ray-config"

	CodeFlarePresentMessage = "" +
		"Failed upgrade: CodeFlare component is present in the cluster. " +
		"It must be uninstalled to proceed with Ray component upgrade.\n" +
		"To uninstall it, you should delete all RayClusters resources from the cluster, " +
		"delete the CodeFlare component resource and recreate the RayClusters."
)
