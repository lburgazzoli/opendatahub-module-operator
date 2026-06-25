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

package config_test

import (
	"testing"
	"testing/fstest"

	. "github.com/onsi/gomega"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-datasciencepipelines-operator/pkg/config"
)

func TestLoadFromFS_Defaults(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformType).To(Equal(config.DefaultPlatformType))
	g.Expect(cfg.PlatformVersion.String()).To(Equal("0.0.0"))
	g.Expect(cfg.ApplicationsNamespace).To(Equal(config.DefaultApplicationsNS))
}

func TestLoadFromFS_ParsesPlatformVersion(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(fstest.MapFS{
		config.KeyPlatformVersion: {Data: []byte("3.5.0")},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformVersion.String()).To(Equal("3.5.0"))
}

func TestLoadFromFS_EmptyPlatformVersionIsZero(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(fstest.MapFS{
		config.KeyPlatformVersion: {Data: []byte("")},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformVersion.String()).To(Equal("0.0.0"))
}

func TestLoadFromFS_InvalidPlatformVersionReturnsError(t *testing.T) {
	g := NewWithT(t)

	_, err := config.LoadFromFS(fstest.MapFS{
		config.KeyPlatformVersion: {Data: []byte("not-a-version")},
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("not-a-version"))
}

func TestLoadFromFS_PlatformType(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(fstest.MapFS{
		config.KeyPlatformType: {Data: []byte(config.PlatformTypeSelfManagedRhoai)},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cfg.PlatformType).To(Equal(config.PlatformTypeSelfManagedRhoai))
}

func TestComponentRelease(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(fstest.MapFS{
		config.KeyPlatformVersion: {Data: []byte("2.1.0")},
	})
	g.Expect(err).NotTo(HaveOccurred())

	rel := cfg.ComponentRelease()
	g.Expect(rel.Name).To(Equal(config.ReleasePlatform))
	g.Expect(rel.Version).To(Equal("2.1.0"))
}

func TestPlatformRelease(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(fstest.MapFS{
		config.KeyPlatformType:    {Data: []byte(config.DefaultPlatformType)},
		config.KeyPlatformVersion: {Data: []byte("2.1.0")},
	})
	g.Expect(err).NotTo(HaveOccurred())

	rel := cfg.PlatformRelease()
	g.Expect(string(rel.Name)).To(Equal(config.DefaultPlatformType))
	g.Expect(rel.Version.String()).To(Equal("2.1.0"))
}

func TestComponentRelease_EmptyVersion(t *testing.T) {
	g := NewWithT(t)

	cfg, err := config.LoadFromFS(nil)
	g.Expect(err).NotTo(HaveOccurred())

	// Zero OperatorVersion serialises as "0.0.0"
	rel := cfg.ComponentRelease()
	g.Expect(rel.Name).To(Equal(config.ReleasePlatform))
	g.Expect(rel.Version).To(Equal("0.0.0"))
}
