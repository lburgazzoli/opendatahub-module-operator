package controller

const (
	// ConditionProvisioned is the primary machine-readable condition that
	// consumers gate on (docs/plan.md §5 status contract). Declared here so
	// schemaclaim, databaseclaim, and databaseprovider can share a single
	// source of truth without cross-importing each other's packages.
	ConditionProvisioned = "Provisioned"

	// ConditionTLSConfiguration tracks whether TLS is disabled, pending, or
	// fully configured for the current resource.
	ConditionTLSConfiguration = "TLSConfiguration"

	ReasonTLSNotEnabled   = "TLSNotEnabled"
	ReasonTLSProvisioning = "TLSProvisioning"
	ReasonTLSConfigured   = "TLSConfigured"
)
