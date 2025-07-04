/*
 * Copyright 2021 The Knative Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package continual

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	eventingduckv1 "knative.dev/eventing/pkg/apis/duck/v1"
	eventing "knative.dev/eventing/pkg/apis/eventing/v1"
	testlib "knative.dev/eventing/test/lib"
	"knative.dev/eventing/test/lib/duck"
	"knative.dev/eventing/test/lib/resources"
	"knative.dev/eventing/test/upgrade/prober/sut"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	pointer "knative.dev/pkg/ptr"
	pkgupgrade "knative.dev/pkg/test/upgrade"

	"knative.dev/eventing-kafka-broker/control-plane/pkg/kafka"
	testingpkg "knative.dev/eventing-kafka-broker/test/pkg"
)

const (
	kafkaBrokerConfigTemplatePath = "test/upgrade/continual/kafka-broker-config.toml"
)

// KafkaBrokerTestOptions holds test options for Kafka Broker tests.
type KafkaBrokerTestOptions struct {
	*TestOptions
	*Broker
	*Triggers
}

func (o *KafkaBrokerTestOptions) setDefaults() {
	if o.TestOptions == nil {
		o.TestOptions = &TestOptions{}
	}
	if o.Broker == nil {
		o.Broker = &Broker{
			Name:  "broker-upgrade",
			Class: kafka.BrokerClass,
		}
	}
	if o.ReplicationOptions == nil {
		o.ReplicationOptions = defaultReplicationOptions()
	}
	if o.RetryOptions == nil {
		o.RetryOptions = defaultRetryOptions()
	}
	if o.Triggers == nil {
		o.Triggers = &Triggers{
			Triggers: sut.Triggers{},
		}
	}
	if o.Prefix == "" {
		o.Prefix = "trigger-upgrade"
	}
	if o.Types == nil {
		o.Types = eventTypes
	}
}

// BrokerTest tests a broker operation in continual manner during the
// whole upgrade and downgrade process asserting that all event are
// propagated well.
func BrokerTest(opts KafkaBrokerTestOptions) pkgupgrade.BackgroundOperation {
	opts.setDefaults()
	return continualVerification(
		"KafkaBrokerContinualTest",
		opts.TestOptions,
		&kafkaBrokerSut{Broker: *opts.Broker, Triggers: *opts.Triggers},
		kafkaBrokerConfigTemplatePath,
	)
}

type kafkaBrokerSut struct {
	Broker
	Triggers
}

func (k kafkaBrokerSut) Deploy(ctx sut.Context, destination duckv1.Destination) interface{} {
	k.deployBroker(ctx)
	url := k.fetchURL(ctx)
	k.deployTriggers(ctx, destination)
	return url
}

func (k kafkaBrokerSut) deployBroker(ctx sut.Context) {
	namespace := ctx.Namespace
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kafka-broker-upgrade-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			kafka.BootstrapServersConfigMapKey:              testingpkg.BootstrapServersPlaintext,
			kafka.DefaultTopicNumPartitionConfigMapKey:      fmt.Sprintf("%d", k.NumPartitions),
			kafka.DefaultTopicReplicationFactorConfigMapKey: fmt.Sprintf("%d", k.ReplicationFactor),
		},
	}
	cm, err := ctx.Kube.CoreV1().ConfigMaps(namespace).Create(ctx.Ctx, cm, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		ctx.T.Fatalf("Failed to create ConfigMap %s/%s: %v", namespace, cm.GetName(), err)
	}

	ctx.CreateBrokerOrFail(k.Name,
		resources.WithConfigForBroker(&duckv1.KReference{
			Kind:       "ConfigMap",
			Namespace:  cm.GetNamespace(),
			Name:       cm.GetName(),
			APIVersion: "v1",
		}),
		resources.WithBrokerClassForBroker(k.Class),
		resources.WithDeliveryForBroker(&eventingduckv1.DeliverySpec{
			Retry:         pointer.Int32(int32(k.RetryCount)),
			BackoffPolicy: &k.BackoffPolicy,
			BackoffDelay:  &k.BackoffDelay,
		}),
	)
}

func (k *kafkaBrokerSut) fetchURL(ctx sut.Context) *apis.URL {
	namespace := ctx.Namespace
	ctx.Log.Debugf("Fetching \"%s\" broker URL for ns %s", k.Name, namespace)

	meta := resources.NewMetaResource(
		k.Name, namespace, testlib.BrokerTypeMeta,
	)
	err := duck.WaitForResourceReady(ctx.Dynamic, meta)
	if err != nil {
		ctx.T.Fatal(err)
	}

	br, err := ctx.Client.Eventing.EventingV1().Brokers(namespace).Get(
		ctx.Ctx, k.Name, metav1.GetOptions{},
	)
	if err != nil {
		ctx.T.Fatal(err)
	}

	url := br.Status.Address.URL
	ctx.Log.Debugf("\"%s\" broker URL for ns %s is %v", k.Name, namespace, url)
	return url
}

func (k *kafkaBrokerSut) deployTriggers(ctx sut.Context, dest duckv1.Destination) {
	for _, eventType := range k.Types {
		name := fmt.Sprintf("%s-%s", k.Prefix, eventType)
		ctx.Log.Debugf("Creating trigger \"%s\" for type %s to route to %#v", name, eventType, dest)
		ctx.CreateTriggerOrFail(
			name,
			resources.WithBroker(k.Name),
			resources.WithAttributesTriggerFilter(eventing.TriggerAnyFilter, eventType, nil),
			resources.WithSubscriberDestination(func(t *eventing.Trigger) duckv1.Destination { return dest }),
		)
	}
}
