// Package handlers provides controller-runtime event handler aliases from the operator-actions-framework.
package handlers

import (
	odhhandlers "github.com/opendatahub-io/operator-actions-framework/controller/handlers"
)

// ToNamed returns an event handler that routes all events to the named singleton instance.
var ToNamed = odhhandlers.ToNamed
