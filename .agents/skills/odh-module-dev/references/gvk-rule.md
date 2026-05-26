# GVK Package Rule

For split modules, `gvk` refers to the module-local package
`pkg/resources/gvk/gvk.go`, not to
`github.com/opendatahub-io/opendatahub-operator/v2/pkg/cluster/gvk`
imported directly from controller or chartgen code.

Use this rule consistently:

1. Create `pkg/resources/gvk/gvk.go` in the module before porting GVK-based
   `OwnsGVK`, `WatchesGVK`, or chartgen logic
2. Re-export upstream GVK values there when upstream already defines them
3. Define module-only GVKs there when upstream has no constant
4. Keep shared chartgen GVKs there too, so the controller and `cmd/chartgen/`
   use the same import path
