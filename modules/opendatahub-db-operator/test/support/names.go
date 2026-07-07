package support

// ManagedDeploymentName is this module's own controller-manager Deployment name
// (config/manager/manager.yaml). Unlike other modules, db-operator has no separate
// third-party operand Deployment to check (docs/plan.md §3) -- this is the operator's
// own pod.
const ManagedDeploymentName = "operator"
