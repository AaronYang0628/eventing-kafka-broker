/*
Copyright 2020 The Knative Authors

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

package v1

import (
	"context"
	"strconv"

	"knative.dev/pkg/ptr"

	"github.com/google/uuid"

	"knative.dev/eventing-kafka-broker/control-plane/pkg/apis/sources/config"
	"knative.dev/pkg/apis"
)

const (
	uuidPrefix = "knative-kafka-source-"

	classAnnotation             = "autoscaling.knative.dev/class"
	minScaleAnnotation          = "autoscaling.knative.dev/minScale"
	maxScaleAnnotation          = "autoscaling.knative.dev/maxScale"
	pollingIntervalAnnotation   = "keda.autoscaling.knative.dev/pollingInterval"
	cooldownPeriodAnnotation    = "keda.autoscaling.knative.dev/cooldownPeriod"
	kafkaLagThresholdAnnotation = "keda.autoscaling.knative.dev/kafkaLagThreshold"
)

// SetDefaults ensures KafkaSource reflects the default values.
func (k *KafkaSource) SetDefaults(ctx context.Context) {
	ctx = apis.WithinParent(ctx, k.ObjectMeta)

	if k.Spec.ConsumerGroup == "" {
		k.Spec.ConsumerGroup = uuidPrefix + uuid.New().String()
	}

	if k.Spec.Consumers == nil {
		k.Spec.Consumers = ptr.Int32(1)
	}

	if k.Spec.InitialOffset == "" {
		k.Spec.InitialOffset = OffsetLatest
	}

	if k.Spec.Ordering == nil {
		deliveryOrdering := Ordered
		k.Spec.Ordering = &deliveryOrdering
	}

	kafkaConfig := config.FromContextOrDefaults(ctx)
	kafkaDefaults := kafkaConfig.KafkaSourceDefaults
	if kafkaDefaults.AutoscalingClass == config.KedaAutoscalingClass {
		if k.Annotations == nil {
			k.Annotations = map[string]string{}
		}
		k.Annotations[classAnnotation] = kafkaDefaults.AutoscalingClass

		// Set all annotations regardless of defaults
		k.Annotations[minScaleAnnotation] = strconv.FormatInt(kafkaDefaults.MinScale, 10)
		k.Annotations[maxScaleAnnotation] = strconv.FormatInt(kafkaDefaults.MaxScale, 10)
		k.Annotations[pollingIntervalAnnotation] = strconv.FormatInt(kafkaDefaults.PollingInterval, 10)
		k.Annotations[cooldownPeriodAnnotation] = strconv.FormatInt(kafkaDefaults.CooldownPeriod, 10)
		k.Annotations[kafkaLagThresholdAnnotation] = strconv.FormatInt(kafkaDefaults.KafkaLagThreshold, 10)
	}

	k.Spec.Sink.SetDefaults(ctx)
	k.Spec.Delivery.SetDefaults(ctx)
}
