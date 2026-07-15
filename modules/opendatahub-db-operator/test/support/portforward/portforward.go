package portforward

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type Forward struct {
	host      string
	localPort int
	stopCh    chan struct{}
	doneCh    <-chan error
	closeOnce sync.Once
}

func StartPod(
	ctx context.Context,
	restCfg *rest.Config,
	namespace string,
	podName string,
	remotePort int,
	opts ...Option,
) (*Forward, error) {
	if restCfg == nil {
		return nil, fmt.Errorf("rest config is nil")
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace is empty")
	}
	if podName == "" {
		return nil, fmt.Errorf("pod name is empty")
	}
	if remotePort <= 0 || remotePort > 65535 {
		return nil, fmt.Errorf("remote port must be between 1 and 65535, got %d", remotePort)
	}

	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt.ApplyTo(&options)
		}
	}
	if err := options.Validate(); err != nil {
		return nil, err
	}

	transport, upgrader, err := spdy.RoundTripperFor(restCfg)
	if err != nil {
		return nil, fmt.Errorf("building spdy round tripper: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating kube clientset: %w", err)
	}

	url := clientset.CoreV1().
		RESTClient().
		Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").
		URL()
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	forwarder, err := portforward.NewOnAddresses(
		dialer,
		options.Addresses,
		[]string{fmt.Sprintf("%d:%d", options.LocalPort, remotePort)},
		stopCh,
		readyCh,
		stdout,
		stderr,
	)
	if err != nil {
		return nil, fmt.Errorf("building port forwarder: %w", err)
	}

	doneCh := make(chan error, 1)
	go func() {
		err := forwarder.ForwardPorts()
		if err == nil && stderr.Len() != 0 {
			err = fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		doneCh <- err
		close(doneCh)
	}()

	select {
	case <-ctx.Done():
		close(stopCh)
		return nil, ctx.Err()
	case err := <-doneCh:
		if err == nil && stderr.Len() != 0 {
			err = fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		if err == nil {
			err = fmt.Errorf("port-forward stopped before becoming ready")
		}
		return nil, err
	case <-readyCh:
	}

	ports, err := forwarder.GetPorts()
	if err != nil {
		close(stopCh)
		return nil, fmt.Errorf("reading forwarded ports: %w", err)
	}
	if len(ports) == 0 {
		close(stopCh)
		return nil, fmt.Errorf("no forwarded ports reported")
	}

	return &Forward{
		host:      options.Addresses[0],
		localPort: int(ports[0].Local),
		stopCh:    stopCh,
		doneCh:    doneCh,
	}, nil
}

func StartService(
	ctx context.Context,
	restCfg *rest.Config,
	namespace string,
	serviceName string,
	remotePort int,
	opts ...Option,
) (*Forward, error) {
	if restCfg == nil {
		return nil, fmt.Errorf("rest config is nil")
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace is empty")
	}
	if serviceName == "" {
		return nil, fmt.Errorf("service name is empty")
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating kube clientset: %w", err)
	}

	service, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("reading service %s/%s: %w", namespace, serviceName, err)
	}
	podName, err := servicePodName(ctx, clientset, service)
	if err != nil {
		return nil, err
	}

	return StartPod(ctx, restCfg, namespace, podName, remotePort, opts...)
}

func (f *Forward) Host() string {
	if f == nil {
		return ""
	}

	return f.host
}

func (f *Forward) Port() int {
	if f == nil {
		return 0
	}

	return f.localPort
}

func (f *Forward) Close(ctx context.Context) error {
	if f == nil {
		return nil
	}

	f.closeOnce.Do(func() {
		close(f.stopCh)
	})
	if f.doneCh == nil {
		return nil
	}

	select {
	case err := <-f.doneCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func servicePodName(
	ctx context.Context,
	clientset kubernetes.Interface,
	service *corev1.Service,
) (string, error) {
	if service == nil {
		return "", fmt.Errorf("service is nil")
	}
	if len(service.Spec.Selector) == 0 {
		return "", fmt.Errorf("service %s/%s has no selector", service.Namespace, service.Name)
	}

	pods, err := clientset.CoreV1().Pods(service.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(),
	})
	if err != nil {
		return "", fmt.Errorf("listing pods for service %s/%s: %w", service.Namespace, service.Name, err)
	}

	return selectReadyPod(pods.Items)
}

func selectReadyPod(pods []corev1.Pod) (string, error) {
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return pod.Name, nil
			}
		}
	}

	return "", fmt.Errorf("no running ready pod found for port-forward")
}
