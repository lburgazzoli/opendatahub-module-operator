package logger

import (
	"bufio"
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

func streamPodLogs(
	ctx context.Context,
	clientset kubernetes.Interface,
	opts StreamOptions,
) error {
	pod, err := waitForPod(ctx, clientset, opts)
	if err != nil {
		return err
	}

	req := clientset.CoreV1().Pods(opts.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: opts.Container,
		Follow:    true,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		return fmt.Errorf("streaming pod %s logs: %w", pod.Name, err)
	}
	defer func() {
		_ = stream.Close()
	}()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		if opts.LoggerFn != nil {
			opts.LoggerFn("%s%s", opts.Prefix, scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("reading pod %s logs: %w", pod.Name, err)
	}

	return nil
}

func waitForPod(
	ctx context.Context,
	clientset kubernetes.Interface,
	opts StreamOptions,
) (*corev1.Pod, error) {
	if clientset == nil {
		return nil, fmt.Errorf("clientset is nil")
	}
	if opts.Name != "" {
		pod, err := clientset.CoreV1().Pods(opts.Namespace).Get(ctx, opts.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("getting pod %s: %w", opts.Name, err)
		}

		return pod, nil
	}

	selector := labels.SelectorFromSet(opts.MatchLabels).String()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		pods, err := clientset.CoreV1().Pods(opts.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return nil, fmt.Errorf("listing pods: %w", err)
		}
		for idx := range pods.Items {
			pod := &pods.Items[idx]
			if pod.Status.Phase == corev1.PodRunning {
				return pod, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
