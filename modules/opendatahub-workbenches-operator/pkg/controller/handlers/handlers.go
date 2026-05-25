// Package handlers provides controller-runtime event handler aliases from the opendatahub-operator.
package handlers

import (
	odhhandlers "github.com/opendatahub-io/opendatahub-operator/v2/pkg/controller/handlers"
)

// ToNamed returns an event handler that routes all events to the named singleton instance.
var ToNamed = odhhandlers.ToNamed
