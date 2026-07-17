package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/rs/xid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres"
	pginstance "github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/pkg/postgres/instance"
	"github.com/lburgazzoli/opendatahub-module-operator/modules/opendatahub-db-operator/test/support"
	odhresources "github.com/opendatahub-io/odh-platform-utilities/pkg/resources"
)

type Instance struct {
	cfg       postgres.Config
	client    postgres.Client
	cli       client.Client
	resources []client.Object
}

const (
	defaultReadyTimeout = 90 * time.Second
	defaultPollInterval = 1 * time.Second
	hostAuthInitDBArgs  = "--auth-host=scram-sha-256"
	applyFieldManager   = "integration-db-harness"
)

func Start(
	ctx context.Context,
	opts ...Option,
) (*Instance, error) {
	options := Options{
		Name:      "integration-db-" + xid.New().String(),
		Namespace: support.IntegrationTestNamespace(),
		Image:     "postgres:16",
	}
	for _, opt := range opts {
		if opt != nil {
			opt.applyOption(&options)
		}
	}

	if err := options.Validate(); err != nil {
		return nil, err
	}

	password, err := postgres.GeneratePassword(24)
	if err != nil {
		return nil, fmt.Errorf("generating integration database password: %w", err)
	}

	data := pginstance.Data{
		Namespace:    options.Namespace,
		ProviderName: options.Name,
		Service: pginstance.Service{
			Name: options.Name,
		},
		PVC: pginstance.PVC{
			Name: options.Name,
			Size: "1Gi",
		},
		InitDB: pginstance.InitDB{
			ConfigMapName: options.Name + "-initdb",
		},
		Postgres: pginstance.Postgres{
			Image: options.Image,
			Envs: []corev1.EnvVar{{
				Name: "POSTGRES_INITDB_ARGS", Value: hostAuthInitDBArgs,
			}},
			AdminSecretName: options.Name + "-admin",
		},
	}

	inst := &Instance{
		cfg: pginstance.AdminConfig(data, password, nil),
		cli: options.Client,
	}

	if err := support.EnsureNamespace(ctx, options.Client, data.Namespace); err != nil {
		return nil, fmt.Errorf("ensuring integration database namespace: %w", err)
	}

	pgRes, err := pginstance.Resources(ctx, data)
	if err != nil {
		return nil, closeStartError(inst, fmt.Errorf("rendering integration database resources: %w", err))
	}

	slices.SortStableFunc(pgRes, compareResources)

	inst.resources = make([]client.Object, 0, len(pgRes)+1)
	inst.resources = append(inst.resources, pginstance.AdminSecret(data, []byte(password), nil))

	for i := range pgRes {
		inst.resources = append(inst.resources, pgRes[i].DeepCopy())
	}

	for i := range inst.resources {
		obj := inst.resources[i]
		if obj == nil {
			return nil, closeStartError(inst, fmt.Errorf("integration database resource %d is nil", i))
		}

		if err := odhresources.Apply(
			ctx,
			options.Client,
			obj,
			client.FieldOwner(applyFieldManager),
		); err != nil {
			return nil, closeStartError(
				inst,
				fmt.Errorf("applying integration database resource %s - %s/%s: %w",
					fmt.Sprintf("%T", obj),
					obj.GetNamespace(),
					obj.GetName(),
					err,
				),
			)
		}
	}

	if err := waitUntilReady(ctx, options.Client, options.ClientFactory, inst.cfg, data); err != nil {
		return nil, closeStartError(inst, err)
	}

	inst.client, err = options.ClientFactory(ctx, inst.cfg)
	if err != nil {
		return nil, closeStartError(inst, fmt.Errorf("opening integration database client: %w", err))
	}

	return inst, nil
}

func (db *Instance) Config() postgres.Config {
	return db.cfg
}

func (db *Instance) Client() postgres.Client {
	return db.client
}

func (db *Instance) Close(ctx context.Context) error {
	if db == nil {
		return nil
	}

	if db.client != nil {
		db.client.Close()
		db.client = nil
	}

	var errs []error
	for i := len(db.resources) - 1; i >= 0; i-- {
		if err := support.DeleteAndWait(ctx, db.cli, db.resources[i]); err != nil {
			errs = append(errs, err)
		}
	}
	db.resources = nil

	return errors.Join(errs...)
}

func waitUntilReady(
	ctx context.Context,
	cli client.Client,
	clientFactory postgres.ClientFactory,
	cfg postgres.Config,
	data pginstance.Data,
) error {
	if err := wait.PollUntilContextTimeout(
		ctx,
		defaultPollInterval,
		defaultReadyTimeout,
		true,
		func(ctx context.Context) (bool, error) {
			statefulSet := &appsv1.StatefulSet{}
			key := client.ObjectKey{
				Name:      data.Service.Name,
				Namespace: data.Namespace,
			}
			if err := cli.Get(ctx, key, statefulSet); err != nil {
				return false, nil
			}
			if statefulSet.Status.ReadyReplicas < 1 {
				return false, nil
			}

			service := &corev1.Service{}
			if err := cli.Get(ctx, key, service); err != nil {
				return false, nil
			}

			pgClient, err := clientFactory(ctx, cfg)
			if err != nil {
				return false, nil
			}
			defer pgClient.Close()

			if err := pgClient.Ping(ctx); err != nil {
				return false, nil
			}

			return true, nil
		},
	); err != nil {
		return fmt.Errorf("waiting for integration database to become ready: %w", err)
	}

	return nil
}

func compareResources(
	left unstructured.Unstructured,
	right unstructured.Unstructured,
) int {
	leftWeight := resourceWeight(left.GetKind())
	rightWeight := resourceWeight(right.GetKind())
	if leftWeight != rightWeight {
		return leftWeight - rightWeight
	}

	if left.GetName() < right.GetName() {
		return -1
	}
	if left.GetName() > right.GetName() {
		return 1
	}

	return 0
}

func resourceWeight(kind string) int {
	switch kind {
	case "ConfigMap":
		return 1
	case "PersistentVolumeClaim":
		return 2
	case "Service":
		return 3
	case "NetworkPolicy":
		return 4
	case "StatefulSet":
		return 5
	default:
		return 10
	}
}

func closeStartError(inst *Instance, cause error) error {
	if cause == nil || inst == nil {
		return cause
	}

	if closeErr := inst.Close(context.Background()); closeErr != nil {
		return errors.Join(cause, closeErr)
	}

	return cause
}
