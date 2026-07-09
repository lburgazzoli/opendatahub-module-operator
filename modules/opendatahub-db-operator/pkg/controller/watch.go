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
	"context"

	metav1 "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// BroadcastListMapper returns a map function that enqueues every object in the
// provided list type whenever the watched dependency changes.
func BroadcastListMapper(
	cli client.Client,
	prototype client.ObjectList,
) handler.MapFunc {
	return func(ctx context.Context, _ client.Object) []reconcile.Request {
		list, ok := prototype.DeepCopyObject().(client.ObjectList)
		if !ok {
			return nil
		}
		if err := cli.List(ctx, list); err != nil {
			return nil
		}

		items, err := metav1.ExtractList(list)
		if err != nil {
			return nil
		}

		reqs := make([]reconcile.Request, 0, len(items))
		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(obj),
			})
		}
		return reqs
	}
}
