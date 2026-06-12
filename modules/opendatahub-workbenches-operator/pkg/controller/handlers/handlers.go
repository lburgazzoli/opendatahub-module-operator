// Package handlers provides controller-runtime event handler aliases from the shared framework.
package handlers

import (
	fwhandlers "github.com/opendatahub-io/odh-platform-utilities/framework/controller/handlers"
)

// ToNamed returns an event handler that routes all events to the named singleton instance.
var ToNamed = fwhandlers.ToNamed
