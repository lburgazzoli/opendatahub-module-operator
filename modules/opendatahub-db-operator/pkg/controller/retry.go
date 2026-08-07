/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"errors"
	"net"
	"syscall"
	"time"

	odherrors "github.com/opendatahub-io/odh-platform-utilities/framework/controller/actions/errors"
)

const connectionRefusedRetryAfter = time.Second

// StopWithQuickRetryIfConnectionRefused returns a quick-requeue StopError when
// err is a connection-refused failure, and nil otherwise.
//
// Detection walks the error chain with errors.As to find a *net.OpError whose
// underlying syscall error is ECONNREFUSED. This is reliable across locales,
// error-wrapping styles, and TLS stacks — unlike string matching on the error
// message, which breaks for non-English systems or non-standard wrappers.
func StopWithQuickRetryIfConnectionRefused(err error) error {
	if err == nil {
		return nil
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.ECONNREFUSED) {
		return odherrors.NewStopErrorW(err).WithRequeueAfter(connectionRefusedRetryAfter)
	}

	return nil
}
