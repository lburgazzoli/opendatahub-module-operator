package resources

import (
	odhcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	frameworkresources "github.com/opendatahub-io/operator-actions-framework/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	SetLabel      = frameworkresources.SetLabel
	SetAnnotation = frameworkresources.SetAnnotation
	GetSingleton  = odhcluster.GetSingleton[client.Object]
	ErrNoInstance = odhcluster.ErrNoInstance
)
