package helm

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type restClientGetter struct {
	restConfig *rest.Config
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	if g == nil || g.restConfig == nil {
		return nil, fmt.Errorf("rest config is nil")
	}

	return rest.CopyConfig(g.restConfig), nil
}

func (g *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	cfg, err := g.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes clientset: %w", err)
	}

	return memory.NewMemCacheClient(clientset.Discovery()), nil
}

func (g *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	return restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient), nil
}

func (g *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	cfg := g.restConfig
	if cfg == nil {
		return clientcmd.NewNonInteractiveClientConfig(
			clientcmdapi.Config{},
			"",
			&clientcmd.ConfigOverrides{},
			nil,
		)
	}

	const (
		clusterName = "test-cluster"
		authName    = "test-user"
		contextName = "test-context"
	)

	clientCfg := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   cfg.Host,
				InsecureSkipTLSVerify:    cfg.Insecure,
				CertificateAuthority:     cfg.CAFile,
				CertificateAuthorityData: cfg.CAData,
				TLSServerName:            cfg.ServerName,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			authName: {
				ClientCertificate:     cfg.CertFile,
				ClientCertificateData: cfg.CertData,
				ClientKey:             cfg.KeyFile,
				ClientKeyData:         cfg.KeyData,
				Token:                 cfg.BearerToken,
				Username:              cfg.Username,
				Password:              cfg.Password,
				Impersonate:           cfg.Impersonate.UserName,
				ImpersonateGroups:     cfg.Impersonate.Groups,
				ImpersonateUserExtra:  cfg.Impersonate.Extra,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: authName,
			},
		},
		CurrentContext: contextName,
	}

	return clientcmd.NewNonInteractiveClientConfig(
		clientCfg,
		contextName,
		&clientcmd.ConfigOverrides{},
		nil,
	)
}
