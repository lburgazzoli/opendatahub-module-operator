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

package claimerrors

import "fmt"

type DatabaseNotFound struct {
	Database string
}

func (e DatabaseNotFound) Error() string {
	return fmt.Sprintf("database %q not found", e.Database)
}

type DatabaseCreateNotAllowed struct {
	Database string
}

func (e DatabaseCreateNotAllowed) Error() string {
	return fmt.Sprintf("creating database %q is not allowed by the provider", e.Database)
}

type SchemaCreateNotAllowed struct {
	Schema string
}

func (e SchemaCreateNotAllowed) Error() string {
	return fmt.Sprintf("creating schema %q is not allowed by the provider", e.Schema)
}
